# FastCommit

A smart command-line tool that generates git commit messages using AI. Unlike other similar tools, FastCommit uniquely adapts to your repository's existing commit style, making it perfect for established codebases.

FastCommit understands that a good commit message needs more than just code change summaries - it captures intention, context, and references that help others understand the change better. That's why it provides a simple way to add this additional context..

## Features

- Automatically generates context-aware commit messages
- Follows your repository's existing commit style
- Supports custom style guides
- Allows adding extra context for better messages
- Works with multiple OpenAI-compatible providers
- Supports commit message amendment

## Installation

clone and build:
```bash
git clone https://github.com/yourusername/fastcommit
cd fastcommit
make build
```

## Setup

You'll need an OpenAI API key to use FastCommit. You can set it up in two ways:

```bash
# Option 1: Environment variable
export OPENAI_API_KEY="your-api-key"

# Option 2: Save permanently
fastcommit --save-key "your-api-key"
```

## Usage

### Basic Usage
```bash
# Generate commit message for staged changes
git add .
fastcommit

# Amend the last commit message
fastcommit --amend

# Dry run (preview without committing)
fastcommit --dry

# Generate message for a specific commit
fastcommit <commit-hash>
```

### Adding Context
Provide additional context to generate better commit messages:

```bash
# Reference issues
fastcommit -c "fixes #123"

# Add performance context
fastcommit -c "improved API response time by 40%"

# Multiple context items
fastcommit -c "urgent hotfix" -c "temporary solution"
```

### Environment Variables
```bash
OPENAI_API_KEY="your-key"      # API key
FASTCOMMIT_DEBUG=true          # Enable debug mode
FASTCOMMIT_MODEL="gpt-4"       # Set default model
OPENAI_BASE_URL="custom-url"   # Use different API endpoint
```