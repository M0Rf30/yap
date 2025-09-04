#!/bin/bash

# YAP i18n Logger Message Automation Script
# This script helps automate the process of finding and converting hardcoded logger messages to use i18n

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}YAP i18n Logger Message Automation${NC}"
echo "===================================="

# Function to scan for hardcoded logger messages
scan_hardcoded_messages() {
    echo -e "${YELLOW}Scanning for hardcoded logger messages...${NC}"
    
    # Create temporary Go file for scanning
    cat > /tmp/i18n_scanner.go << 'EOF'
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type LoggerMessage struct {
	File     string
	Line     int
	Method   string
	Message  string
	I18nKey  string
	Category string
}

func main() {
	messages := scanForMessages()
	for _, msg := range messages {
		fmt.Printf("%s:%d:%s:\"%s\" -> %s\n", 
			msg.File, msg.Line, msg.Method, msg.Message, msg.I18nKey)
	}
}

func scanForMessages() []LoggerMessage {
	var messages []LoggerMessage
	
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(path, ".go") || 
		   strings.Contains(path, "_test.go") ||
		   strings.Contains(path, "vendor/") ||
		   strings.Contains(path, ".git/") {
			return nil
		}
		
		fileMessages := scanFile(path)
		messages = append(messages, fileMessages...)
		return nil
	})
	
	return messages
}

func scanFile(filename string) []LoggerMessage {
	var messages []LoggerMessage
	
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return messages
	}
	
	ast.Inspect(node, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "logger" {
					method := sel.Sel.Name
					if method == "Info" || method == "Debug" || method == "Warn" || method == "Error" {
						if len(call.Args) > 0 {
							if lit, ok := call.Args[0].(*ast.BasicLit); ok {
								message := strings.Trim(lit.Value, `"`)
								if !strings.Contains(message, "i18n.T") {
									pos := fset.Position(call.Pos())
									msg := LoggerMessage{
										File:     filename,
										Line:     pos.Line,
										Method:   method,
										Message:  message,
										I18nKey:  generateI18nKey(filename, message),
										Category: getCategory(filename),
									}
									messages = append(messages, msg)
								}
							}
						}
					}
				}
			}
		}
		return true
	})
	
	return messages
}

func generateI18nKey(filename, message string) string {
	category := getCategory(filename)
	key := strings.ToLower(message)
	key = regexp.MustCompile(`[^a-z0-9\s]`).ReplaceAllString(key, "")
	key = regexp.MustCompile(`\s+`).ReplaceAllString(key, "_")
	key = strings.Trim(key, "_")
	if len(key) > 40 {
		key = key[:40]
	}
	if key == "" {
		key = "message"
	}
	return fmt.Sprintf("logger.%s.%s", category, key)
}

func getCategory(filename string) string {
	if strings.Contains(filename, "pkg/project") { return "project" }
	if strings.Contains(filename, "pkg/shell") { return "shell" }
	if strings.Contains(filename, "pkg/download") { return "download" }
	if strings.Contains(filename, "pkg/files") { return "files" }
	if strings.Contains(filename, "pkg/platform") { return "platform" }
	if strings.Contains(filename, "cmd/yap/command") { return "command" }
	if strings.Contains(filename, "pkg/builders") { return "builder" }
	if strings.Contains(filename, "pkg/git") { return "git" }
	if strings.Contains(filename, "pkg/pkgbuild") { return "pkgbuild" }
	if strings.Contains(filename, "pkg/core") { return "core" }
	if strings.Contains(filename, "pkg/archive") { return "archive" }
	if strings.Contains(filename, "pkg/crypto") { return "crypto" }
	if strings.Contains(filename, "pkg/graph") { return "graph" }
	return "misc"
}
EOF

    cd "$PROJECT_ROOT"
    go run /tmp/i18n_scanner.go > /tmp/scan_results.txt
    
    echo -e "${GREEN}Scan complete! Results saved to /tmp/scan_results.txt${NC}"
    echo "Found $(wc -l < /tmp/scan_results.txt) hardcoded logger messages"
    
    # Show summary by category
    echo -e "\n${YELLOW}Summary by category:${NC}"
    awk -F: '{gsub(/.*logger\./, "", $4); gsub(/\..*/, "", $4); print $4}' /tmp/scan_results.txt | sort | uniq -c | sort -rn
}

# Function to generate i18n keys for English locale
generate_english_keys() {
    echo -e "${YELLOW}Generating English i18n keys...${NC}"
    
    if [ ! -f /tmp/scan_results.txt ]; then
        echo -e "${RED}No scan results found. Run scan first.${NC}"
        return 1
    fi
    
    echo "# Auto-generated logger i18n keys for English locale" > /tmp/en_keys.yaml
    echo "# Add these to pkg/i18n/locales/en.yaml" >> /tmp/en_keys.yaml
    echo "" >> /tmp/en_keys.yaml
    
    # Group by category
    categories=($(awk -F: '{gsub(/.*logger\./, "", $4); gsub(/\..*/, "", $4); print $4}' /tmp/scan_results.txt | sort -u))
    
    for category in "${categories[@]}"; do
        echo "# ${category^} messages" >> /tmp/en_keys.yaml
        grep "logger\.$category\." /tmp/scan_results.txt | while IFS=: read -r file line method message key; do
            clean_message=$(echo "$message" | sed 's/"//g')
            clean_key=$(echo "$key" | sed 's/"//g')
            echo "- id: \"$clean_key\"" >> /tmp/en_keys.yaml
            echo "  translation: \"$clean_message\"" >> /tmp/en_keys.yaml
        done
        echo "" >> /tmp/en_keys.yaml
    done
    
    echo -e "${GREEN}English keys generated in /tmp/en_keys.yaml${NC}"
}

# Function to add i18n import to Go files that need it
add_i18n_imports() {
    echo -e "${YELLOW}Adding i18n imports to Go files...${NC}"
    
    if [ ! -f /tmp/scan_results.txt ]; then
        echo -e "${RED}No scan results found. Run scan first.${NC}"
        return 1
    fi
    
    # Get unique files that need i18n import
    files=($(awk -F: '{print $1}' /tmp/scan_results.txt | sort -u))
    
    for file in "${files[@]}"; do
        if [ -f "$PROJECT_ROOT/$file" ]; then
            # Check if i18n import already exists
            if ! grep -q "github.com/M0Rf30/yap/v2/pkg/i18n" "$PROJECT_ROOT/$file"; then
                echo "Adding i18n import to $file"
                
                # Find the import block and add i18n import
                sed -i '/github\.com\/M0Rf30\/yap\/v2\/pkg\//i\\t"github.com/M0Rf30/yap/v2/pkg/i18n"' "$PROJECT_ROOT/$file"
            fi
        fi
    done
    
    echo -e "${GREEN}i18n imports added${NC}"
}

# Function to convert hardcoded strings to i18n.T() calls
convert_to_i18n() {
    echo -e "${YELLOW}Converting hardcoded strings to i18n.T() calls...${NC}"
    
    if [ ! -f /tmp/scan_results.txt ]; then
        echo -e "${RED}No scan results found. Run scan first.${NC}"
        return 1
    fi
    
    while IFS=: read -r file line method message key; do
        if [ -f "$PROJECT_ROOT/$file" ]; then
            clean_message=$(echo "$message" | sed 's/"//g')
            clean_key=$(echo "$key" | sed 's/"//g')
            
            echo "Converting $file:$line: \"$clean_message\" -> i18n.T(\"$clean_key\")"
            
            # Use sed to replace the hardcoded string with i18n.T() call
            escaped_message=$(echo "$clean_message" | sed 's/[[\.*^$()+?{|]/\\&/g')
            sed -i "${line}s/\"$escaped_message\"/i18n.T(\"$clean_key\")/g" "$PROJECT_ROOT/$file"
        fi
    done < /tmp/scan_results.txt
    
    echo -e "${GREEN}Conversion complete${NC}"
}

# Function to generate basic translations for other locales
generate_basic_translations() {
    echo -e "${YELLOW}Generating basic translations for other locales...${NC}"
    
    if [ ! -f /tmp/en_keys.yaml ]; then
        echo -e "${RED}No English keys found. Run generate-keys first.${NC}"
        return 1
    fi
    
    # Italian translations (basic)
    echo "# Auto-generated Italian translations" > /tmp/it_keys.yaml
    sed 's/translation: "downloading"/translation: "scaricamento"/' /tmp/en_keys.yaml | \
    sed 's/translation: "executing command"/translation: "esecuzione comando"/' | \
    sed 's/translation: "command execution failed"/translation: "esecuzione comando fallita"/' | \
    sed 's/translation: "failed to"/translation: "impossibile"/' | \
    sed 's/translation: "package installed"/translation: "pacchetto installato"/' \
    >> /tmp/it_keys.yaml
    
    # Russian translations (basic)
    echo "# Auto-generated Russian translations" > /tmp/ru_keys.yaml
    sed 's/translation: "downloading"/translation: "загрузка"/' /tmp/en_keys.yaml | \
    sed 's/translation: "executing command"/translation: "выполнение команды"/' | \
    sed 's/translation: "command execution failed"/translation: "выполнение команды не удалось"/' | \
    sed 's/translation: "failed to"/translation: "не удалось"/' | \
    sed 's/translation: "package installed"/translation: "пакет установлен"/' \
    >> /tmp/ru_keys.yaml
    
    # Chinese translations (basic) 
    echo "# Auto-generated Chinese translations" > /tmp/zh_keys.yaml
    sed 's/translation: "downloading"/translation: "下载中"/' /tmp/en_keys.yaml | \
    sed 's/translation: "executing command"/translation: "执行命令"/' | \
    sed 's/translation: "command execution failed"/translation: "命令执行失败"/' | \
    sed 's/translation: "failed to"/translation: "失败"/' | \
    sed 's/translation: "package installed"/translation: "包已安装"/' \
    >> /tmp/zh_keys.yaml
    
    echo -e "${GREEN}Basic translations generated:${NC}"
    echo "  - Italian: /tmp/it_keys.yaml"
    echo "  - Russian: /tmp/ru_keys.yaml"  
    echo "  - Chinese: /tmp/zh_keys.yaml"
}

# Function to run tests after conversion
run_tests() {
    echo -e "${YELLOW}Running tests to verify changes...${NC}"
    
    cd "$PROJECT_ROOT"
    
    # Run specific package tests that are likely to use logger
    echo "Testing pkg/source..."
    go test ./pkg/source -v
    
    echo "Testing pkg/project..."
    go test ./pkg/project -v -run TestGlobalVariables
    
    echo "Checking i18n integrity..."
    go run cmd/i18n-tool/main.go check
    
    echo -e "${GREEN}Tests completed${NC}"
}

# Function to show usage
show_usage() {
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  scan                 - Scan for hardcoded logger messages"
    echo "  generate-keys        - Generate English i18n keys"
    echo "  generate-translations - Generate basic translations for all locales"
    echo "  add-imports          - Add i18n imports to Go files"
    echo "  convert              - Convert hardcoded strings to i18n.T() calls"
    echo "  test                 - Run tests to verify changes"
    echo "  all                  - Run all steps in sequence"
    echo "  clean                - Clean temporary files"
    echo ""
    echo "Example workflow:"
    echo "  $0 scan"
    echo "  $0 generate-keys"
    echo "  $0 add-imports"
    echo "  $0 convert"
    echo "  $0 test"
}

# Function to clean temporary files
clean_temp() {
    echo -e "${YELLOW}Cleaning temporary files...${NC}"
    rm -f /tmp/i18n_scanner.go /tmp/scan_results.txt /tmp/en_keys.yaml /tmp/it_keys.yaml /tmp/ru_keys.yaml /tmp/zh_keys.yaml
    echo -e "${GREEN}Cleanup complete${NC}"
}

# Main script logic
case "${1:-}" in
    scan)
        scan_hardcoded_messages
        ;;
    generate-keys)
        generate_english_keys
        ;;
    generate-translations)
        generate_basic_translations
        ;;
    add-imports)
        add_i18n_imports
        ;;
    convert)
        convert_to_i18n
        ;;
    test)
        run_tests
        ;;
    all)
        scan_hardcoded_messages
        generate_english_keys
        generate_basic_translations
        add_i18n_imports
        convert_to_i18n
        run_tests
        ;;
    clean)
        clean_temp
        ;;
    *)
        show_usage
        exit 1
        ;;
esac

echo -e "${GREEN}Done!${NC}"