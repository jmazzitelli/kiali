# Running Frontend Cypress Tests with Integration Framework

This guide explains how to use the Kiali Integration Test Framework to run the frontend Cypress test suite.

## Overview

The Kiali frontend includes a comprehensive Cypress test suite with 49+ feature files covering:
- Graph display and manipulation
- Multi-cluster scenarios
- Authentication and authorization
- Istio configuration management
- Performance and load testing
- Ambient mesh functionality

The Integration Framework provides a complete testing environment that automatically:
- Sets up a Kubernetes cluster (KinD or Minikube)
- Installs Istio, Kiali, and Prometheus
- Starts the frontend development server
- Runs the Cypress tests
- Cleans up resources

## Quick Start

### Prerequisites

1. **Docker/Podman** - For running KinD clusters
2. **kubectl** - Kubernetes CLI
3. **Node.js & Yarn/NPM** - For frontend development server
4. **Go 1.19+** - For building the integration framework

### Basic Usage

```bash
# Navigate to integration framework directory
cd kiali-integration-framework

# Run the complete test suite
./run-frontend-cypress.sh
```

This script will:
1. Build the integration framework
2. Create a KinD cluster
3. Install Istio, Kiali, and Prometheus
4. Start the frontend development server
5. Run all Cypress tests
6. Clean up resources

## Configuration

### Frontend-Specific Configuration

The `frontend-cypress-config.yaml` is specifically configured for frontend testing:

```yaml
tests:
  frontend-cypress:
    type: cypress
    config:
      cypress:
        baseUrl: "http://localhost:3000"     # Frontend dev server
        specPattern: "**/*.feature"          # Cucumber feature files
        workingDir: "../kiali/frontend"      # Frontend directory
        headless: true                      # Run without UI
        defaultCommandTimeout: 40000        # Extended timeouts
        tags: ["@error-rates-app", "@core"] # Test tags to run
```

### Customizing Test Execution

#### Running Specific Test Tags

```bash
# Run only core functionality tests
./kiali-test test run frontend-cypress --config frontend-cypress-config.yaml --tags "@core"

# Run multi-cluster tests
./kiali-test test run frontend-cypress --config frontend-cypress-config.yaml --tags "@multi-cluster"

# Run ambient mesh tests
./kiali-test test run frontend-cypress --config frontend-cypress-config.yaml --tags "@ambient"
```

#### Running Specific Feature Files

```bash
# Run only graph display tests
./kiali-test test run frontend-cypress --config frontend-cypress-config.yaml --spec "cypress/integration/featureFiles/graph_display.feature"

# Run multiple specific tests
./kiali-test test run frontend-cypress --config frontend-cypress-config.yaml --spec "cypress/integration/featureFiles/graph_*.feature"
```

#### Headed Mode (with browser UI)

```yaml
# In your config file, set headless to false
cypress:
  headless: false
  browser: "chrome"  # or "firefox", "edge"
```

## Manual Step-by-Step Execution

If you prefer to run steps manually:

### 1. Build the Framework

```bash
cd kiali-integration-framework
make build
```

### 2. Create Cluster

```bash
./kiali-test cluster create --config frontend-cypress-config.yaml
```

### 3. Install Components

```bash
./kiali-test component install --config frontend-cypress-config.yaml
```

### 4. Start Frontend Server

```bash
cd ../kiali/frontend
yarn start  # or npm start
```

### 5. Run Tests

```bash
cd ../../kiali-integration-framework
./kiali-test test run frontend-cypress --config frontend-cypress-config.yaml
```

### 6. Cleanup

```bash
./kiali-test cluster delete --config frontend-cypress-config.yaml
```

## Available Test Tags

The frontend Cypress tests use tags to categorize scenarios:

- `@core` - Core functionality tests
- `@error-rates-app` - Tests using the error-rates demo application
- `@bookinfo-app` - Tests using the bookinfo demo application
- `@multi-cluster` - Multi-cluster specific tests
- `@ambient` - Ambient mesh tests
- `@ossmc` - OpenShift Service Mesh Console tests

## Troubleshooting

### Common Issues

#### Frontend Server Won't Start

**Problem**: `yarn start` or `npm start` fails in frontend directory

**Solution**:
```bash
cd ../kiali/frontend
# Install dependencies if needed
yarn install  # or npm install
# Clear cache and try again
yarn start
```

#### Cypress Can't Connect to Frontend

**Problem**: Tests fail with connection errors

**Solution**:
1. Verify frontend server is running on port 3000
2. Check firewall settings
3. Ensure no port conflicts

#### Cluster Creation Fails

**Problem**: KinD cluster creation fails

**Solution**:
```bash
# Check Docker/Podman is running
docker ps

# Clean up any existing clusters
kind delete clusters --all

# Try again
./kiali-test cluster create --config frontend-cypress-config.yaml
```

#### Component Installation Fails

**Problem**: Istio/Kiali/Prometheus installation fails

**Solution**:
```bash
# Check cluster status
./kiali-test cluster status --config frontend-cypress-config.yaml

# Check component status
./kiali-test component status --config frontend-cypress-config.yaml

# Reinstall if needed
./kiali-test component install --config frontend-cypress-config.yaml
```

### Debug Mode

Enable verbose logging for troubleshooting:

```bash
# Set in config file
global:
  logLevel: "debug"
  verbose: true
```

Or use CLI flags:
```bash
./kiali-test --verbose --log-level debug test run frontend-cypress --config frontend-cypress-config.yaml
```

## Performance Optimization

### For CI/CD Pipelines

Use these settings for faster execution in CI:

```yaml
cypress:
  browser: "electron"  # Fastest option
  headless: true
  defaultCommandTimeout: 10000  # Shorter timeouts
  video: false  # Disable video recording
```

### For Development

Use these settings for interactive development:

```yaml
cypress:
  headless: false
  browser: "chrome"
  defaultCommandTimeout: 60000  # Longer timeouts
  video: true  # Enable video recording for debugging
```

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: Frontend Cypress Tests

on:
  pull_request:
    paths:
      - 'frontend/**'
      - 'kiali-integration-framework/**'

jobs:
  cypress:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.19'

      - name: Setup Node.js
        uses: actions/setup-node@v3
        with:
          node-version: '18'

      - name: Run Frontend Cypress Tests
        run: |
          cd kiali-integration-framework
          ./run-frontend-cypress.sh
```

## Test Results and Artifacts

The framework generates:
- **Test Results**: JSON and HTML reports
- **Screenshots**: On test failures
- **Videos**: Optional video recordings
- **Logs**: Comprehensive execution logs

Artifacts are stored in:
```
kiali-integration-framework/
├── test-results/
│   ├── cypress-frontend/
│   │   ├── screenshots/
│   │   ├── videos/
│   │   └── reports/
│   └── logs/
```

## Contributing

When adding new frontend Cypress tests:

1. Follow the existing naming convention: `*.feature`
2. Add appropriate tags for categorization
3. Update this documentation if needed
4. Test with the integration framework before committing

## Support

For issues or questions:
1. Check the troubleshooting section above
2. Review the integration framework logs
3. Check Cypress-specific documentation
4. Create an issue in the Kiali repository
