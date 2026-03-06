# Universal Email Analytics (UEA)

**Universal Email Analytics (UEA)** is a powerful, self-hosted web application for deep-dive email analytics. It connects to your existing email accounts via IMAP and provides a comprehensive dashboard to explore your data, discover trends, and gain insights from your communication history.

## Key Features

*   **Unified Dashboard**: Aggregate and analyze email data from multiple accounts in a single professional-grade interface powered by the `nexus-shell` framework.
*   **Interactive Analytics**: Explore your data with interactive heatmaps (powered by `@nivo/calendar`), dynamic donut charts, and topic treemaps. Drill down with deep cross-filtering between dates, senders, and topics.
*   **Intelligent Mailbox**: Seamlessly pivot from high-level analytics to a high-performance email feed filtered precisely by your dashboard selections.
*   **Visual AI Agent Builder**: Visually design and deploy custom AI agents powered by the Eino framework using an interactive node-based canvas (`reactflow`).
*   **AI-Assisted Workflows**: Leverage the power of large language models (LLMs) to summarize threads, draft responses, and more.
*   **Privacy-First**: Your data is stored locally, and no email content is ever sent to a third party without your explicit consent.
*   **Cross-Platform**: UEA is available for Windows, macOS, and Linux.

## Getting Started

UEA is distributed as a single, zero-dependency binary.

### Download Release

Download the latest release for your operating system and run the executable:

```bash
./uea
```

This will start the web server. You can then access the UEA dashboard by opening your web browser and navigating to `http://localhost:8080`.

### Build from Source

To build UEA from source, ensure you have Go 1.21+ and Node.js 20+ installed, then run:

```bash
make build
```

This will produce the `bin/uea` executable. You can start the server in the foreground with:

```bash
make start --foreground
```

## CLI Management

UEA also includes a powerful command-line interface (CLI) for managing your accounts and performing other administrative tasks.

*   `uea account`: Add, list, remove, or verify connections to your email accounts.
*   `uea doctor`: Run diagnostics to check the health of your UEA installation.
*   `uea maintenance`: Perform maintenance tasks such as re-indexing your data.
*   `uea backup`: Create and manage backups of your UEA data.

For more information on the CLI, run `uea --help`.

## Contributing

We welcome contributions from the community! If you're interested in contributing to UEA, please see our [Development Guide](development.md) for more information.
