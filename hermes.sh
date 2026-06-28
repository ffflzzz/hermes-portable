#!/bin/bash
# Hermes Diagnostic Client - Linux/Mac launcher
clear

echo "========================================"
echo "  HERMES DIAGNOSTIC CLIENT"
echo "========================================"
echo ""

# Check if python3 is available
if ! command -v python3 &> /dev/null; then
    echo "ERROR: Python 3 is not installed!"
    echo ""
    echo "Please install Python 3.10+:"
    echo "  Ubuntu/Debian: sudo apt install python3 python3-pip"
    echo "  macOS: brew install python3"
    echo ""
    read -p "Press Enter to exit..."
    exit 1
fi

# Check if requests is installed
python3 -c "import requests" 2>/dev/null
if [ $? -ne 0 ]; then
    echo "Installing required library (requests)..."
    pip3 install requests --user --quiet 2>/dev/null || pip install requests --quiet 2>/dev/null
    if [ $? -ne 0 ]; then
        echo "Failed to install requests."
        echo "Run: pip3 install requests"
        read -p "Press Enter to exit..."
        exit 1
    fi
fi

# Run the client
cd "$(dirname "$0")"
python3 client.py

read -p "Press Enter to exit..."
