#!/bin/bash
# GitHub Actions Workflow Validation Script
# Validates YAML syntax, structure, and common issues

set -e

echo "🔍 Validating GitHub Actions workflows..."
echo ""

# Check if workflows directory exists
if [ ! -d ".github/workflows" ]; then
    echo "❌ No .github/workflows directory found"
    exit 1
fi

# Count workflows
workflow_count=$(find .github/workflows -name "*.yml" | wc -l)
if [ "$workflow_count" -eq 0 ]; then
    echo "❌ No workflow files found"
    exit 1
fi

echo "📊 Found $workflow_count workflow files"
echo ""

# YAML syntax validation
echo "📋 YAML Syntax Validation:"
yaml_errors=0
for file in .github/workflows/*.yml; do
    if [ -f "$file" ]; then
        filename=$(basename "$file")
        echo -n "  $filename: "
        
        if python3 -c "import yaml; yaml.safe_load(open('$file'))" 2>/dev/null; then
            echo "✅ Valid"
        else
            echo "❌ Invalid"
            yaml_errors=$((yaml_errors + 1))
        fi
    fi
done

if [ $yaml_errors -gt 0 ]; then
    echo ""
    echo "❌ Found $yaml_errors YAML syntax errors"
    exit 1
fi

echo ""
echo "🔧 GitHub Actions Structure Validation:"
structure_errors=0
for file in .github/workflows/*.yml; do
    if [ -f "$file" ]; then
        filename=$(basename "$file")
        echo "  $filename:"
        
        # Check required fields
        for field in "name" "on" "jobs"; do
            if grep -q "^$field:" "$file"; then
                echo "    ✅ Has '$field' field"
            else
                echo "    ❌ Missing '$field' field"
                structure_errors=$((structure_errors + 1))
            fi
        done
    fi
done

if [ $structure_errors -gt 0 ]; then
    echo ""
    echo "❌ Found $structure_errors structure errors"
    exit 1
fi

echo ""
echo "🔒 Security Check:"
# Check for deprecated syntax
deprecated_found=0
for file in .github/workflows/*.yml; do
    if [ -f "$file" ]; then
        filename=$(basename "$file")
        
        if grep -q "::set-output" "$file"; then
            echo "  ⚠️  $filename uses deprecated ::set-output command"
            deprecated_found=$((deprecated_found + 1))
        fi
    fi
done

if [ $deprecated_found -eq 0 ]; then
    echo "  ✅ No deprecated syntax found"
fi

echo ""
echo "✨ All validations passed! Workflows are ready for use."