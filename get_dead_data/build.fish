#!/usr/bin/fish

# This script builds the three executables for the get-dead-data tool.
# It should be run from within the get_dead_data directory.

echo "Building main console application (get_dead_data)..."
go build -o get_dead_data .
if test $status -ne 0
    echo "Error: Failed to build main application."
    exit 1
end

echo "Building captcha GUI (captcha_gui)..."
go build -tags=captcha_gui -o captcha_gui .
if test $status -ne 0
    echo "Error: Failed to build captcha GUI."
    exit 1
end

echo "Building config GUI (config_gui)..."
go build -tags=config_gui -o config_gui .
if test $status -ne 0
    echo "Error: Failed to build config GUI."
    exit 1
end

echo ""
echo "Build complete!"
echo "The following executables have been created in the current directory:"
echo "- get_dead_data"
echo "- captcha_gui"
echo "- config_gui"
