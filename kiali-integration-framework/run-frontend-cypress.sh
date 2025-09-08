#!/bin/bash

# Script to run Kiali Frontend Cypress Tests using the Integration Framework
# This script demonstrates how to use the framework to run the frontend Cypress test suite

set -e

echo "🧪 Running Kiali Frontend Cypress Tests with Integration Framework"
echo "================================================================="

# Check if we're in the right directory
if [ ! -f "frontend-cypress-config.yaml" ]; then
    echo "❌ Error: frontend-cypress-config.yaml not found. Please run from kiali-integration-framework directory."
    exit 1
fi

# Check if Kiali frontend directory exists
if [ ! -d "../kiali/frontend" ]; then
    echo "❌ Error: Kiali frontend directory not found at ../kiali/frontend"
    echo "   Please ensure the Kiali repository is cloned alongside this integration framework."
    exit 1
fi

echo "✅ Configuration files found"
echo "✅ Kiali frontend directory found"

# Build the integration framework
echo ""
echo "🔨 Building integration framework..."
make build

if [ $? -ne 0 ]; then
    echo "❌ Build failed"
    exit 1
fi

echo "✅ Build successful"

# Check if cluster is already running
echo ""
echo "🔍 Checking for existing test cluster..."
if ./kiali-test cluster status --config frontend-cypress-config.yaml >/dev/null 2>&1; then
    echo "ℹ️  Test cluster already exists"
    CLUSTER_EXISTS=true
else
    CLUSTER_EXISTS=false
fi

# Start cluster if needed
if [ "$CLUSTER_EXISTS" = false ]; then
    echo "🚀 Starting test cluster..."
    ./kiali-test cluster create --config frontend-cypress-config.yaml
else
    echo "✅ Using existing test cluster"
fi

# Install components
echo ""
echo "📦 Installing required components (Istio, Kiali, Prometheus)..."
./kiali-test component install --config frontend-cypress-config.yaml

# Wait for components to be ready
echo ""
echo "⏳ Waiting for components to be ready..."
sleep 30

# Check component status
echo "📊 Checking component status..."
./kiali-test component status --config frontend-cypress-config.yaml

# Start the frontend development server (in background)
echo ""
echo "🌐 Starting Kiali frontend development server..."
cd ./frontend

# Check if yarn is available, otherwise use npm
if command -v yarn &> /dev/null; then
    echo "📦 Using yarn to start frontend server..."
    yarn start &
else
    echo "📦 Using npm to start frontend server..."
    npm start &
fi

FRONTEND_PID=$!
echo "✅ Frontend server started (PID: $FRONTEND_PID)"

# Wait for frontend to be ready
echo "⏳ Waiting for frontend server to be ready..."
sleep 60

# Go back to integration framework directory
cd ../../kiali-integration-framework

# Run Cypress tests
echo ""
echo "🧪 Running Cypress tests..."
./kiali-test test run frontend-cypress --config frontend-cypress-config.yaml

TEST_EXIT_CODE=$?

# Cleanup
echo ""
echo "🧹 Cleaning up..."

# Kill frontend server
if kill -0 $FRONTEND_PID 2>/dev/null; then
    echo "🛑 Stopping frontend server..."
    kill $FRONTEND_PID
fi

# Cleanup cluster if we created it
if [ "$CLUSTER_EXISTS" = false ]; then
    echo "🗑️  Cleaning up test cluster..."
    ./kiali-test cluster delete --config frontend-cypress-config.yaml
fi

echo ""
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "✅ All tests passed successfully!"
else
    echo "❌ Some tests failed. Check the output above for details."
fi

echo "================================================================="
echo "🎉 Frontend Cypress testing completed"
echo ""

exit $TEST_EXIT_CODE
