# Caption Studio (In active development/not ready for use) 🚧

An AI-powered image captioning and annotation tool built with Go and React.

## Features

- **AI Image Captioning**: Generate captions using Google's Gemini AI model
- **Project Management**: Organize images into projects with metadata tracking
- **Batch Processing**: Handle multiple images with progress tracking
- **Export Capabilities**: Export annotated datasets in various formats
- **Web Interface**: Modern React frontend with real-time updates

## Tech Stack

- **Backend**: Go with SQLite database
- **Frontend**: React + TypeScript + Tailwind CSS
- **AI Integration**: Google Gemini for image captioning
- **Build Tool**: Task (Taskfile.yml)

## Getting Started

### Prerequisites

- Go 1.21+
- Node.js 18+
- pnpm

### Installation

1. Clone the repository
2. Install dependencies:

   ```bash
   cd frontend && pnpm install
   cd ../backend && go mod download
   ```

3. Set up environment variables (see `.env.template`)

4. Run in development mode:

   ```bash
   task dev
   ```

## Development

The project uses:

- **Backend**: Go server on port 8080
- **Frontend**: Vite dev server on port 5173
- **Database**: SQLite with automatic migrations

## License

GPL v3
