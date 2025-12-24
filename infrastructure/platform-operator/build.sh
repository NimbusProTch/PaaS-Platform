#!/bin/bash

# Build script for platform operator
# This script must be run from the project root to include charts

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}Building Platform Operator...${NC}"

# Check we're in the right directory
if [ ! -f "infrastructure/platform-operator/Dockerfile" ]; then
    echo -e "${RED}Error: Must run this script from project root${NC}"
    echo "Current directory: $(pwd)"
    exit 1
fi

# Check charts exist
if [ ! -d "charts" ]; then
    echo -e "${YELLOW}Warning: charts/ directory not found${NC}"
fi

if [ ! -d "platform-charts" ]; then
    echo -e "${YELLOW}Warning: platform-charts/ directory not found${NC}"
fi

# Build image (using project root as context)
IMAGE_NAME=${1:-"platform-operator:latest"}

echo -e "${GREEN}Building with context: $(pwd)${NC}"
echo -e "${GREEN}Image name: ${IMAGE_NAME}${NC}"

docker build \
    -f infrastructure/platform-operator/Dockerfile \
    -t "${IMAGE_NAME}" \
    .

echo -e "${GREEN}✓ Build complete: ${IMAGE_NAME}${NC}"
