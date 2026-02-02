package sync

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"regexp" // Added
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
	textplain "github.com/emersion/go-textwrapper" // Corrected import and aliased
	"github.com/google/uuid"
	"github.com/user/uea/internal/account"
	"github.com/user/uea/internal/hasher"
	"github.com/user/uea/internal/message"
	"github.com/user/uea/internal/store"
)

const DefaultMaxHostConnections = 5

// SyncManager manages synchronization for multiple accounts, handling concurrency limits.
type SyncManager struct {
	mu                sync.Mutex
	hostConnections   map[string]chan struct{}
	MaxHostConnections int
}

// NewSyncManager creates a new SyncManager.
func NewSyncManager(maxHostConnections int) *SyncManager {
	if maxHostConnections <= 0 {
		maxHostConnections = DefaultMaxHostConnections
	}
	return &SyncManager{
		hostConnections:   make(map[string]chan struct{}),
		MaxHostConnections: maxHostConnections,
	}
}

// getHostConnectionLimiter returns the connection limiter for a given host.
// It creates one if it doesn't exist.
func (sm *SyncManager) getHostConnectionLimiter(host string) chan struct{} {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if limiter, ok := sm.hostConnections[host]; ok {
		return limiter
	}
	limiter := make(chan struct{}, sm.MaxHostConnections)
	sm.hostConnections[host] = limiter
	return limiter
}

// StartSync initiates the synchronization process for a given account.
func (sm *SyncManager) StartSync(acc *account.Account) {
	limiter := sm.getHostConnectionLimiter(acc.Host)
	limiter <- struct{}{} // Acquire a connection slot
	defer func() {
		<-limiter // Release the connection slot
	}()

	log.Printf("Starting sync for account %s on host %s", acc.ID, acc.Host)

	c, err := ConnectIMAP(acc)
	if err != nil {
		log.Printf("Failed to connect for account %s: %v", acc.ID, err)
		return
	}
	defer c.Logout()

	mailboxes, err := ListMailboxes(c, "", "*")
	if err != nil {
		log.Printf("Failed to list mailboxes for account %s: %v", acc.ID, err)
		return
	}

	totalMessagesSynced := 0
	for _, mb := range mailboxes {
		if strings.Contains(mb.Name, "INBOX") || strings.Contains(mb.Name, "Sent") { // Only sync INBOX and Sent for now
			log.Printf("Syncing mailbox: %s for account: %s", mb.Name, acc.ID)
			numSynced, err := sm.syncMailbox(c, acc, mb.Name)
			if err != nil {
				log.Printf("Failed to sync mailbox %s for account %s: %v", mb.Name, acc.ID, err)
			}
			totalMessagesSynced += numSynced
		}
	}

	log.Printf("Account %s: Synced %d messages across all relevant mailboxes.", acc.ID, totalMessagesSynced)
}

// syncMailbox fetches and processes messages from a single mailbox.
func (sm *SyncManager) syncMailbox(c *client.Client, acc *account.Account, mailboxName string) (int, error) {
	// Select mailbox
	mbox, err := c.Select(mailboxName, false)
	if err != nil {
		return 0, fmt.Errorf("failed to select mailbox %s: %w", mailboxName, err)
	}

	// Get last sync state for this mailbox
	mailboxID := fmt.Sprintf("%s-%s", acc.ID, mailboxName)
	syncState, err := store.GetMailboxSyncState(mailboxID)
	if err != nil {
		return 0, fmt.Errorf("failed to get mailbox sync state for %s: %w", mailboxID, err)
	}
	if syncState == nil {
		syncState = &store.MailboxSyncState{
			ID:        mailboxID,
			AccountID: acc.ID,
			Name:      mailboxName,
			LastUID:   0,
			LastMODSEQ: 0,
		}
	}

	// Fetch messages
	fromUID := syncState.LastUID + 1
	if fromUID > mbox.Messages { // No new messages
		log.Printf("No new messages in mailbox %s for account %s.", mailboxName, acc.ID)
		return 0, nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(fromUID, imap.MaxSeqNum) // Fetch from last UID + 1 to end

	// Items to fetch
	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchInternalDate,
		imap.FetchModSeq,
		imap.FetchRFC822Header,
		imap.FetchRFC822Size,
		"BODY[]", // Fetch full body including MIME parts
		imap.FetchUID,
	}

	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.Fetch(seqset, items, messages)
	}()

	fetchedCount := 0
	var newLastUID uint32 = syncState.LastUID
	var newLastMODSEQ uint64 = syncState.LastMODSEQ

	for imapMsg := range messages {
		// Update last UID and MODSEQ
		if imapMsg.UID > newLastUID {
			newLastUID = imapMsg.UID
		}
		if imapMsg.ModSeq > newLastMODSEQ {
			newLastMODSEQ = imapMsg.ModSeq
		}

		parsedMsg, err := parseIMAPMessage(acc.ID, imapMsg)
		if err != nil {
			log.Printf("Error parsing message UID %d for account %s, mailbox %s: %v", imapMsg.UID, acc.ID, mailboxName, err)
			continue
		}

		// Deduplication logic
		existsByMessageID, err := store.MessageExistsByMessageID(parsedMsg.MessageID)
		if err != nil {
			log.Printf("Error checking existence by MessageID %s: %v", parsedMsg.MessageID, err)
			continue
		}
		if existsByMessageID {
			log.Printf("Message with MessageID %s already exists, skipping. (UID %d)", parsedMsg.MessageID, parsedMsg.UID)
			continue
		}

		existsByContentHash, err := store.MessageExistsByContentHash(parsedMsg.ContentHash)
		if err != nil {
			log.Printf("Error checking existence by ContentHash %s: %v", parsedMsg.ContentHash, err)
			continue
		}
		if existsByContentHash {
			log.Printf("Message with ContentHash %s already exists, skipping. (UID %d)", parsedMsg.ContentHash, parsedMsg.UID)
			continue
		}

		// Save message to store
		if err := store.SaveMessage(parsedMsg); err != nil {
			log.Printf("Error saving message ID %s to store: %v", parsedMsg.ID, err)
			continue
		}
		fetchedCount++
	}

	if err := <-done; err != nil {
		return fetchedCount, fmt.Errorf("failed to fetch messages from %s: %w", mailboxName, err)
	}

	// Update mailbox sync state
	syncState.LastUID = newLastUID
	syncState.LastMODSEQ = newLastMODSEQ
	if err := store.SaveMailboxSyncState(syncState); err != nil {
		log.Printf("Failed to save mailbox sync state for %s: %v", mailboxID, err)
	}

	log.Printf("Fetched %d new messages from mailbox %s for account %s.", fetchedCount, mailboxName, acc.ID)
	return fetchedCount, nil
}

// parseIMAPMessage converts an imap.Message into a message.Message struct.
func parseIMAPMessage(accountID string, imapMsg *imap.Message) (*message.Message, error) {
	msg := &message.Message{
		ID:        uuid.New().String(), // Generate a unique ID for our system
		AccountID: accountID,
		UID:       imapMsg.UID,
		Flags:     imapMsg.Flags,
		Size:      imapMsg.Size,
		InternalDate: imapMsg.InternalDate,
	}

	if imapMsg.Envelope != nil {
		msg.Subject = imapMsg.Envelope.Subject
		msg.From = mail.FormatAddressList(imapMsg.Envelope.From)
		msg.To = mail.FormatAddressList(imapMsg.Envelope.To)
		msg.Cc = mail.FormatAddressList(imapMsg.Envelope.Cc)
		msg.Bcc = mail.FormatAddressList(imapMsg.Envelope.Bcc)
		msg.Date = imapMsg.Envelope.Date
		msg.MessageID = imapMsg.Envelope.MessageID
	}

	// Extract message body and headers
	if b := imapMsg.GetBody("BODY[]"); b != nil {
		header, err := mail.ReadHeader(b)
		if err != nil {
			return nil, fmt.Errorf("failed to read message header: %w", err)
		}
		// Store raw headers
		headerBytes := new(bytes.Buffer)
		header.WriteTo(headerBytes)
		msg.Header = headerBytes.Bytes()


		mediaType, params, err := header.ContentType()
		if err != nil {
			log.Printf("Error getting content type for message UID %d: %v", imapMsg.UID, err)
			// Proceed without content type, will try to parse as text
		}

		// Read actual body
		bodyBuf := new(bytes.Buffer)
		if _, err := io.Copy(bodyBuf, b); err != nil {
			return nil, fmt.Errorf("failed to copy message body for UID %d: %w", imapMsg.UID, err)
		}
		fullBody := bodyBuf.Bytes()

		if strings.HasPrefix(mediaType, "multipart/") {
			mr := mail.NewReader(bytes.NewReader(fullBody))
			for {
				p, err := mr.NextPart()
				if err == io.EOF {
					break // No more parts
				}
				if err != nil {
					log.Printf("Error reading multipart part for UID %d: %v", imapMsg.UID, err)
					continue
				}

				partMediaType, _, _ := p.Header.ContentType()
				if partMediaType == "text/plain" && msg.Body == "" {
					bodyBytes, _ := io.ReadAll(p.Body)
					msg.Body = string(bodyBytes)
				} else if partMediaType == "text/html" && msg.HTMLBody == "" {
					htmlBytes, _ := io.ReadAll(p.Body)
					msg.HTMLBody = string(htmlBytes)
				}
				// TODO: Handle attachments later
			}
		} else if mediaType == "text/plain" {
			msg.Body = string(fullBody)
		} else if mediaType == "text/html" {
			msg.HTMLBody = string(fullBody)
		} else {
			// Fallback: try to extract text from any content type
			reader := textplain.NewReader(bytes.NewReader(fullBody))
			bodyBytes, _ := io.ReadAll(reader)
			msg.Body = string(bodyBytes)
		}
	} else if r := imapMsg.GetBody("RFC822.TEXT"); r != nil { // Fallback for servers that don't directly give BODY[]
		log.Printf("Falling back to RFC822.TEXT for message UID %d", imapMsg.UID)
		bodyBytes, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read RFC822.TEXT body for UID %d: %w", imapMsg.UID, err)
		}
		msg.Body = string(bodyBytes)
		// No HTML body for this fallback
	}


	// Use text body for normalization and hashing
	// If HTMLBody exists and plain Body is empty, try to convert HTML to plain text for normalization
	bodyToHash := msg.Body
	if bodyToHash == "" && msg.HTMLBody != "" {
		// A simple heuristic to get some text from HTML for hashing
		// This is not a full HTML to text converter, just a basic attempt.
		// For proper conversion, a dedicated library would be needed.
		bodyToHash = stripHTMLTags(msg.HTMLBody)
	}

	msg.NormalizedBody = hasher.NormalizeAndHashSHA256(bodyToHash)
	msg.ContentHash = msg.NormalizedBody // For now, ContentHash is the hash of NormalizedBody

	return msg, nil
}

// stripHTMLTags is a very basic helper to remove HTML tags.
// This is not robust and should be replaced by a proper HTML parser/converter.
func stripHTMLTags(html string) string {
	re := regexp.MustCompile("<[^>]*>")
	return re.ReplaceAllString(html, "")
}


// ConnectIMAP establishes a connection to the IMAP server and authenticates the user.
func ConnectIMAP(acc *account.Account) (*client.Client, error) {
	var c *client.Client
	var err error

	addr := fmt.Sprintf("%s:%d", acc.Host, acc.Port)

	if acc.SSL {
		c, err = client.DialTLS(addr, &tls.Config{InsecureSkipVerify: true})
	} else {
		c, err = client.Dial(addr)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to IMAP server: %w", err)
	}

	log.Printf("Connected to IMAP server: %s", addr)

	// Login
	if err := c.Login(acc.User, acc.Password); err != nil {
		c.Logout()
		return nil, fmt.Errorf("failed to login to IMAP server: %w", err)
	}

	log.Printf("Logged in as %s", acc.User)
	return c, nil
}

// ListMailboxes lists all mailboxes for the authenticated user.
func ListMailboxes(c *client.Client, delim, pattern string) ([]*imap.MailboxInfo, error) {
	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.List(delim, pattern, mailboxes)
	}()

	var mboxes []*imap.MailboxInfo
	for m := range mailboxes {
		mboxes = append(mboxes, m)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("failed to list mailboxes: %w", err)
	}

	log.Printf("Listed %d mailboxes", len(mboxes))
	return mboxes, nil
}

