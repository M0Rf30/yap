#!/bin/bash

# extract_messages.sh - Extract i18n messages from source code

echo "Extracting i18n messages from source code..."

# This is a placeholder script. In a real implementation, you would:
# 1. Scan all .go files for i18n.T() calls
# 2. Extract the message IDs
# 3. Update the en.yaml file with new messages while preserving existing translations

echo "This script is a placeholder for the Go implementation in scripts/extract_messages.go"
echo "To extract messages, run: go run scripts/extract_messages.go ."

# For now, let's just show what the Go script would do
echo ""
echo "Messages found in source code:"
grep -r "i18n.T(" --include="*.go" . | grep -o '"[^"]*"' | sort | uniq