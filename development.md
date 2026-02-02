# Developer On-ramp: Universal Email Analytics (UEA)

This document provides a comprehensive guide for developers who want to contribute to the Universal Email Analytics (UEA) project.

## 1. Architecture Overview

UEA is a web application with a Golang backend and a ReactJS frontend.

### 1.1. Backend (Golang)

The backend is responsible for:

*   **Multi-Account Sync Engine**: A sophisticated worker pool architecture that manages concurrency on a per-host basis. It uses a stateful incremental sync to fetch new headers or flags, and a content-aware hashing algorithm for deduplication.
*   **Data Persistence & Hybrid Search**: A hybrid search architecture that combines a lexical layer (FTS5) and a semantic layer (Vector Index) for fast and accurate search results.
*   **API & AI Gateway**: A unified interface for multiple AI backends, with support for streaming responses using Server-Sent Events (SSE).
*   **CLI Management Suite**: A powerful administrative tool for managing accounts, running diagnostics, and performing maintenance tasks.

### 1.2. Frontend (ReactJS)

The frontend is a modern ReactJS application that provides a rich and interactive user experience. It uses a Master-Detail-Filter layout and includes a variety of widgets and views for exploring and analyzing email data.

## 2. Getting Started

### 2.1. Prerequisites

*   Go 1.21+
*   Node.js 20.x and npm
*   A C compiler (for `sqlite-vss`)

### 2.2. Installation

1.  **Clone the repository:**

    ```bash
    git clone https://github.com/your-username/uea.git
    cd uea
    ```

2.  **Install frontend dependencies:**

    ```bash
    cd frontend
    npm install
    cd ..
    ```

3.  **Build the backend:**

    ```bash
    go build .
    ```

### 2.3. Running the application

To run the application in development mode, you'll need to start the backend and frontend separately.

1.  **Start the backend:**

    ```bash
    ./uea
    ```

2.  **Start the frontend:**

    ```bash
    cd frontend
    npm run dev
    ```

The UEA dashboard will be available at `http://localhost:3000`.

## 3. Development

### 3.1. Backend

The backend code is located in the root of the repository.

*   **API Endpoints**: The API endpoints are defined in `api.go`.
*   **Sync Engine**: The sync engine is implemented in `sync.go`.
*   **Database**: The database schema is defined in `db.go`.

### 3.2. Frontend

The frontend code is located in the `frontend` directory.

*   **Components**: The React components are located in `frontend/src/components`.
*   **Views**: The main views of the application are located in `frontend/src/views`.
*   **API Client**: The API client is located in `frontend/src/api`.

## 4. Testing

### 4.1. Backend

The backend includes a suite of integration tests that use a mock IMAP server to verify the functionality of the sync engine. To run the tests, use the following command:

```bash
go test ./...
```

### 4.2. Frontend

The frontend uses Jest and React Testing Library for testing. To run the tests, use the following command:

```bash
cd frontend
npm test
```

## 5. CI/CD

The project uses GitHub Actions for CI/CD. The workflow is defined in `.github/workflows/ci.yml`. The workflow is triggered on every push to the `main` branch and on every tagged release. It builds the application for all target architectures and runs the tests.

## 6. Contributing

We welcome contributions from the community! If you're interested in contributing to UEA, please follow these steps:

1.  Fork the repository.
2.  Create a new branch for your feature or bug fix.
3.  Make your changes and commit them with a descriptive commit message.
4.  Push your changes to your fork.
5.  Open a pull request to the `main` branch of the original repository.
