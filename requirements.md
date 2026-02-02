# **Project: Universal Email Analytics (UEA) \- Requirements Document**

## **1\. Introduction**

The **Universal Email Analytics (UEA)** application is a high-performance, self-hosted web application built with a robust **Golang** backend and a modern **ReactJS** frontend. Designed as a standalone, comprehensive dashboard, UEA facilitates the aggregation and exploration of email data across disparate providers via the **IMAP protocol**.

By centralizing data into a local-first environment, UEA empowers users to perform deep-dive analytics—such as trend discovery, social network mapping, and semantic content search—without sacrificing privacy. The application is tailored for power users, researchers, and professionals who manage high-velocity inboxes and require more than what standard webmail clients offer in terms of data visibility and automated insight.

## **2\. Backend Requirements (Golang)**

### **2.1. Multi-Account Sync Engine**

* **Intelligent Worker Pool:** Implement a sophisticated worker pool architecture that manages concurrency on a per-host basis. For example, while the engine can handle 50 concurrent Goroutines, it must limit connections to a single provider (e.g., imap.gmail.com) to a maximum of 10 to respect server-side rate limits and prevent temporary IP blacklisting.  
* **Stateful Incremental Sync:** Utilize IMAP UIDs and MODSEQ (where available) to track synchronization state. The engine should only fetch new headers or flags since the last recorded high-water mark, significantly reducing bandwidth and processing overhead for multi-year archives.  
* **Deduplication Logic:** Implement a content-aware hashing algorithm (e.g., SHA-256) on normalized message bodies and unique Message-ID headers. This ensures that a single email CC'd to multiple managed accounts or moved between folders is treated as a single entity in the analytics layer.  
* **Credential Vault:** All IMAP and SMTP credentials must be encrypted using AES-256-GCM. The encryption key is derived from the user's master passphrase using a high-cost KDF (like Argon2id), ensuring that even if the SQLite file is compromised, the credentials remain secure.

### **2.2. Data Persistence & Hybrid Search**

* **Optimized SQLite Core:** The database must use Write-Ahead Logging (WAL) and synchronous "NORMAL" mode to balance performance and data integrity. Tables for messages, participants, and threads should be highly normalized to facilitate rapid joins during cross-filtering.  
* **The Hybrid Search Architecture:**  
  * **Lexical Layer (FTS5):** Leverage SQLite's FTS5 extension to provide lightning-fast, exact keyword matching. This layer handles specific queries like from:jdoe@example.com, has:attachment, and standard boolean searches ("project alpha" AND NOT "draft").  
  * **Semantic Layer (Vector Index):** Integrate a Go-native vector library or the sqlite-vss extension. Every message body is transformed into a high-dimensional vector (384 or 768 dimensions) using a local embedding model (e.g., all-MiniLM-L6-v2).  
  * **Rank Fusion:** Implement **Reciprocal Rank Fusion (RRF)** to synthesize results. If a user searches for "Travel plans," FTS5 finds messages containing the word "Travel," while the vector index finds messages about "flights," "hotels," and "itineraries," merging them into a single, relevant results list.  
* **Chunking Strategy:** For long emails (e.g., newsletters or long threads), the system must split text into overlapping chunks (e.g., 512 tokens with a 50-token overlap) to ensure that semantic meaning is captured even for content buried deep in a message.

### **2.3. API & AI Gateway**

* **Materialized Analytics Views:** To ensure the UI remains responsive, complex analytical queries (like topic prevalence or sender volume) should not be calculated on-the-fly. The backend must maintain materialized summary tables that are updated asynchronously during sync cycles.  
* **LLM Abstraction Layer:** Provide a unified interface for multiple AI backends. Users can choose between local execution (via **Ollama** or **llama.cpp** sidecars) for maximum privacy, or high-performance remote APIs (OpenAI, Anthropic, Gemini).  
* **Streaming Responses:** The API should support Server-Sent Events (SSE) for the "Bullet-to-Draft" feature, allowing the UI to render the AI-generated response in real-time.

### **2.4. CLI Management Suite**

The uea binary serves as both the web server and a powerful administrative tool:

* **uea account**: Commands to add (--host \--user \--pass), list, remove, or verify connections.  
* **uea doctor**: A comprehensive diagnostic suite that checks local disk health, database indices, LLM connectivity, and IMAP reachability.  
* **uea maintenance**: Commands for reindex-vectors (to upgrade embedding models) and vacuum (to reclaim disk space).  
* **uea backup**:  
  * **Atomic Snapshot:** Uses the sqlite3\_backup API to create a consistent file copy while the application is running.  
  * **Granular Extraction:** A utility to export specific threads or subsets of messages to standardized formats like .eml or .json.

## **3\. Design & User Interface**

### **3.1. General Interface Architecture**

The application adopts a **Master-Detail-Filter** layout.

* **Global Sidebar:** Provides high-level navigation between the Dashboard, Search, Social Graph, and Settings.  
* **Dynamic Filter Bar:** A persistent top-level bar that aggregates all active filters. It supports "Breadcrumb Filtering," where users can see exactly which constraints are narrowing their view (e.g., Account: Personal \[x\] \+ Topic: Finance \[x\]).

### **3.2. Primary Views & Layouts**

#### **3.2.1. The Analytics Pulse (Main Dashboard)**

The "Pulse" is the primary engine for data discovery. It is composed of interactive widgets that communicate via a shared state.

* **Temporal Volume Widget:** A bar chart showing message density over time. Drag-and-drop selection on the chart allows users to "zoom in" on a specific time period.  
* **Sender/Domain Donut Chart:** A breakdown of where mail is coming from. Users can click a domain (e.g., @github.com) to instantly see all notifications from that service.  
* **Topic Treemap:** Uses size and color to represent the volume and sentiment of AI-discovered topics.

#### **3.2.2. The Unified Message List (The Feed)**

* **High-Performance Feed:** Implements React Virtualization to handle scrolling through hundreds of thousands of entries with zero lag.  
* **Contextual Actions:** Quick-action buttons on each list item allow for "Single-Click Search for Related" or "Extract Attachments."  
* **Smart Snippets:** Instead of just the first line of an email, the list shows an AI-generated 1-sentence summary that highlights the core "ask" or "info" in the message.

#### **3.2.3. Thread Focus View**

* **Conversational UI:** Threads are reconstructed chronologically and rendered as a chat interface, stripping away redundant headers and signatures to focus on the dialogue.  
* **Intelligence Side-Panel:** While a user reads, this panel displays "Social Insights" (e.g., "This sender is in your Top 5% of contacts") and "Contextual Links" (e.g., links to previous threads regarding the same topic).  
* **AI Quick-Compose:** A dedicated text area for the **Bullet-to-Draft** workflow.

## **4\. Workflows**

### **4.1. Cross-Filtering & Discovery**

1. User clicks a "Spike" in the Volume Chart (January 2024).  
2. The Sender Chart updates to show who was active in January.  
3. User clicks "Topic: Real Estate" in the Treemap.  
4. The Message List now only shows real-estate-related emails from January 2024\.  
5. This "Drill-down" approach allows users to find specific needles in the haystack without typing a single search query.

### **4.2. AI-Assisted Response (Bullet-to-Draft)**

* **The Workflow:** A user reads a complex inquiry and types three bullet points into the quick reply box: "Can't make Monday," "Available Tuesday 2 PM," "Send the PDF first."  
* **The Generation:** The user clicks **Synthesize**. The system sends the last 5 messages of the thread \+ the 3 bullets \+ a "Persona Profile" (e.g., Professional) to the LLM.  
* **The Result:** The AI generates a 3-paragraph email that gracefully declines the Monday meeting, proposes the Tuesday slot, and requests the document. The user reviews and clicks **Send**.

### **4.3. Secure Cloud Backup & Restore**

* **Encryption:** The system generates a unique 256-bit encryption key from the master passphrase.  
* **The Backup:** Data is compressed, encrypted, and streamed to an S3-compatible bucket (AWS, MinIO, or Cloudflare R2).  
* **The Restore:** Users can view a "Timeline of Snapshots." They can choose to restore the entire DB to a new machine or "Deep-Dive" into a snapshot to find and restore a single deleted message or attachment.

## **5\. Non-Functional Requirements**

### **5.1. Performance & Scalability**

* **Latency Targets:** Lexical searches must return in \<100ms. Semantic searches, involving vector distance calculations, must return in \<400ms for datasets of up to 500k messages.  
* **Resource Efficiency:** The Go backend must utilize a "Buffer Pool" for SQLite operations to minimize disk I/O, keeping total system RAM usage under 1GB during idle background syncing.

### **5.2. Installation & Privacy**

* **Zero-Dependency Binary:** The application is distributed as a single executable for Windows, macOS (Intel/Silicon), and Linux. All HTML, CSS, and JS assets are embedded using go:embed.  
* **Privacy First:** All vector embeddings and topic models are generated locally. No email content is ever sent to a third party unless the user explicitly configures a remote LLM API (like OpenAI).

## **6\. Development & Quality**

* **CI/CD Pipeline:** Automated GitHub Actions trigger builds for all target architectures on every tagged release.  
* **Integration Testing:** Uses a "Mock IMAP Server" to verify that the sync engine handles edge cases like connection drops, malformed headers, and large attachment handling.  
* **Observability:** Implement internal metrics (Prometheus format) for tracking sync speed, search latency, and database growth.