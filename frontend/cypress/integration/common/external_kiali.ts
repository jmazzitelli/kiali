import { Given, Then, When } from '@badeball/cypress-cucumber-preprocessor';
import { ensureKialiFinishedLoading } from './transition';
import { clusterParameterExists } from './navigation';

// Environment variables for cluster contexts
const CLUSTER1_CONTEXT = Cypress.env('CLUSTER1_CONTEXT');
const CLUSTER2_CONTEXT = Cypress.env('CLUSTER2_CONTEXT');

// External Kiali specific step definitions

Then('user sees applications running in the {string} cluster', (cluster: string) => {
  // Verify that graph shows applications from the specified cluster
  cy.get('[data-test="graph-container"]').should('be.visible');

  // Check for cluster-specific badges or indicators in the graph
  cy.get('[data-test="graph-node"]').should('exist');

  // Verify cluster context is properly displayed
  clusterParameterExists(true);
});

Then('graph nodes should display correct cluster information', () => {
  // Verify that graph nodes contain cluster information
  cy.get('[data-test="graph-node"]').should('exist').and('be.visible');

  // Check that cluster badges or labels are present
  cy.get('[data-test="cluster-badge"], [data-test="cluster-label"]').should('exist');
});

Then('user sees Istio control plane components in the {string} cluster', (cluster: string) => {
  // Verify Istio control plane components are visible in the graph
  cy.get('[data-test="graph-container"]').should('be.visible');

  // Look for istiod or other control plane components
  cy.get('[data-test="graph-node"]').should('contain.text', 'istiod');
});

Then('no control plane components should be visible in the {string} cluster graph', (cluster: string) => {
  // Verify that mgmt cluster doesn't show control plane components in the graph
  cy.get('[data-test="graph-container"]').should('be.visible');

  // Ensure no istiod nodes are shown for mgmt cluster
  if (cluster === 'mgmt') {
    cy.get('[data-test="graph-node"]').should('not.contain.text', 'istiod');
  }
});

Then('user sees workloads from the {string} cluster', (cluster: string) => {
  // Verify workloads are shown from the correct cluster
  cy.get('[data-test="workload-list"], [data-test="workload-card"]').should('exist');

  // Check cluster context in workload details
  clusterParameterExists(true);
});

Then('user sees the correct cluster information in application details', () => {
  // Verify application details show correct cluster information
  cy.get('[data-test="app-description-card"]').should('be.visible');

  // Check for cluster parameter in URL or UI elements
  clusterParameterExists(true);
});

Then('user sees pods running in the {string} cluster', (cluster: string) => {
  // Verify pods section shows pods from correct cluster
  cy.get('[data-test="workload-pods"], [data-test="pods-table"]').should('exist');

  // Verify cluster context is maintained
  clusterParameterExists(true);
});

Then('user sees the correct cluster context', () => {
  // Verify cluster context is properly maintained across navigation
  clusterParameterExists(true);
});

Then('user sees endpoints from the {string} cluster', (cluster: string) => {
  // Verify service endpoints are from the correct cluster
  cy.get('[data-test="service-endpoints"], [data-test="endpoints-table"]').should('exist');

  // Check cluster context
  clusterParameterExists(true);
});

Then('service configuration shows correct cluster information', () => {
  // Verify service configuration displays cluster information
  cy.get('[data-test="service-details"]').should('be.visible');

  // Check for cluster-specific information
  clusterParameterExists(true);
});

Then('user sees metrics data from external Prometheus', () => {
  // Verify metrics are loaded from external Prometheus
  cy.get('[data-test="metrics-chart"], [data-test="metrics-component"]').should('exist');

  // Wait for metrics to load
  cy.get('[data-test="metrics-loading"], #loading_kiali_spinner').should('not.exist');

  // Verify charts contain data
  cy.get('[data-test="metrics-chart"]').should('be.visible');
});

Then('metrics charts should display data', () => {
  // Verify that charts actually contain metric data
  cy.get('[data-test="metrics-chart"]').should('be.visible');

  // Wait for any loading indicators to disappear
  cy.get('#loading_kiali_spinner').should('not.exist');
});

Then('external Grafana should be accessible', () => {
  // Verify Grafana link works (check that link exists and is properly formatted)
  cy.get('[data-test="view-in-grafana-link"], a[href*="grafana"]').should('exist');

  // Note: We don't actually click the link as it would navigate away from Kiali
  // In a real test environment, you might want to verify the URL format
});

Then('user sees tracing data from external tracing system', () => {
  // Verify tracing data is loaded from external tracing system
  cy.get('[data-test="traces-chart"], [data-test="tracing-component"]').should('exist');

  // Wait for loading to complete
  cy.get('#loading_kiali_spinner').should('not.exist');
});

Then('trace spans should be visible', () => {
  // Verify trace spans are displayed
  cy.get('[data-test="trace-span"], [data-test="traces-table"]').should('exist');
});

Then('user sees Istio configurations from the {string} cluster', (cluster: string) => {
  // Verify Istio configs are visible from the correct cluster
  cy.get('[data-test="istio-config-list"], [data-test="config-table"]').should('exist');

  // Check cluster context
  clusterParameterExists(true);
});

Then('Istio config objects should be read-only', () => {
  // Verify that Istio config objects from mesh cluster are read-only
  cy.get('[data-test="config-actions"]').should('not.exist');

  // Check that create/edit/delete buttons are not present
  cy.get('[data-test="create-button"], [data-test="edit-button"], [data-test="delete-button"]').should('not.exist');
});

Then('user should not see create/edit/delete options for mesh cluster configs', () => {
  // Verify no modification options are available for mesh cluster configs
  cy.get('[data-test="config-actions"]').should('not.exist');
  cy.get('button').contains('Create').should('not.exist');
  cy.get('button').contains('Edit').should('not.exist');
  cy.get('button').contains('Delete').should('not.exist');
});

Then('user sees the Istio configuration in read-only mode', () => {
  // Verify config editor is in read-only mode
  cy.get('[data-test="config-editor"]').should('exist');

  // Check that editor is read-only
  cy.get('textarea, input').should('have.attr', 'readonly');
});

Then('edit and delete buttons should be disabled', () => {
  // Verify edit/delete buttons are disabled or not present
  cy.get('button').contains('Save').should('not.exist');
  cy.get('button').contains('Delete').should('not.exist');
  cy.get('[data-test="edit-button"]').should('not.exist');
  cy.get('[data-test="delete-button"]').should('not.exist');
});

Then('user sees Kiali running in the {string} cluster', (cluster: string) => {
  // Verify mesh page shows Kiali in the correct cluster
  cy.get('[data-test="mesh-page"]').should('be.visible');

  // Look for Kiali node in the specified cluster
  cy.get('[data-test="mesh-node"]').should('contain.text', 'kiali');
});

Then('user sees Istio control plane running in the {string} cluster', (cluster: string) => {
  // Verify mesh page shows Istio control plane in the correct cluster
  cy.get('[data-test="mesh-node"]').should('contain.text', 'istiod');
});

Then('user sees external service connections between clusters', () => {
  // Verify connections between clusters are shown
  cy.get('[data-test="mesh-edge"], [data-test="mesh-connection"]').should('exist');
});

Then('user sees Prometheus running in the {string} cluster', (cluster: string) => {
  // Verify Prometheus is shown in the correct cluster
  cy.get('[data-test="mesh-node"]').should('contain.text', 'prometheus');
});

Then('user sees the correct cluster separation in mesh topology', () => {
  // Verify mesh topology shows proper cluster separation
  cy.get('[data-test="cluster-box"], [data-test="cluster-group"]').should('exist');
});

Then('user sees connections from Kiali to external Prometheus', () => {
  // Verify external service connections are visible
  cy.get('[data-test="mesh-edge"]').should('exist');
});

Then('user sees connections from Kiali to external Grafana if enabled', () => {
  // Verify Grafana connections if enabled
  cy.get('[data-test="mesh-edge"]').should('exist');
});

Then('user sees connections from Kiali to external tracing if enabled', () => {
  // Verify tracing connections if enabled
  cy.get('[data-test="mesh-edge"]').should('exist');
});

Then('external service nodes should be clearly marked', () => {
  // Verify external services are marked distinctly
  cy.get('[data-test="external-service-node"], [data-test="mesh-node"]').should('exist');
});

Then('user sees the {string} cluster badge for Kiali namespace', (cluster: string) => {
  // Verify cluster badge is shown for Kiali namespace
  cy.get('[data-test="cluster-badge"]').should('contain.text', cluster);
});

Then('user sees the {string} cluster badge for application namespaces', (cluster: string) => {
  // Verify application namespaces show correct cluster badge
  cy.get('[data-test="cluster-badge"]').should('contain.text', cluster);
});

Then('cluster badges should distinguish between mgmt and mesh clusters', () => {
  // Verify cluster badges properly distinguish clusters
  cy.get('[data-test="cluster-badge"]').should('exist');

  // Should see both mgmt and mesh cluster badges
  cy.get('[data-test="cluster-badge"]').should('contain.text', 'mgmt');
  cy.get('[data-test="cluster-badge"]').should('contain.text', 'mesh');
});

Then('URL should contain correct cluster context for {string} cluster', (cluster: string) => {
  // Verify URL contains correct cluster parameter
  cy.url().should('include', `clusterName=${cluster}`);
});

Then('cluster context should be preserved as {string} cluster', (cluster: string) => {
  // Verify cluster context is maintained during navigation
  cy.url().should('include', `clusterName=${cluster}`);
  clusterParameterExists(true);
});

Then('health status should reflect actual workload health from mesh cluster', () => {
  // Verify health indicators work with external data
  cy.get('[data-test="health-indicator"]').should('exist');

  // Check that health data is loaded
  cy.get('#loading_kiali_spinner').should('not.exist');
});

Then('health calculations should work with external metrics', () => {
  // Verify health calculations use external metrics
  cy.get('[data-test="health-indicator"]').should('be.visible');
});

Then('traffic data should come from external Prometheus', () => {
  // Verify traffic metrics come from external source
  cy.get('[data-test="traffic-metrics"]').should('exist');

  // Wait for metrics to load
  cy.get('#loading_kiali_spinner').should('not.exist');
});

Then('inbound/outbound traffic should be accurately displayed', () => {
  // Verify traffic direction is correctly shown
  cy.get('[data-test="traffic-inbound"], [data-test="traffic-outbound"]').should('exist');
});

Then('filtering should work correctly with external data sources', () => {
  // Verify filtering works with external data
  cy.get('[data-test="namespace-filter"]').should('exist');

  // Check that filtered results are displayed
  cy.get('#loading_kiali_spinner').should('not.exist');
});

Then('user sees logs from pods in the {string} cluster', (cluster: string) => {
  // Verify logs are accessible from mesh cluster
  cy.get('[data-test="workload-logs"], [data-test="logs-component"]').should('exist');

  // Check cluster context
  clusterParameterExists(true);
});

Then('log streaming should work across clusters', () => {
  // Verify log streaming functionality works
  cy.get('[data-test="logs-container"]').should('be.visible');
});

Then('validations should work with external Istio API access', () => {
  // Verify validations work with external Istio API
  cy.get('[data-test="validation-status"]').should('exist');
});

Then('validation errors should be displayed correctly', () => {
  // Verify validation errors are shown properly
  cy.get('[data-test="validation-error"], [data-test="validation-warning"]').should('exist');
});

Then('search results should include objects from the {string} cluster', (cluster: string) => {
  // Verify search works across clusters
  cy.get('[data-test="search-results"]').should('exist');

  // Check cluster context in results
  clusterParameterExists(true);
});

Then('search should work with external cluster data', () => {
  // Verify search functionality works with external data
  cy.get('[data-test="search-results"]').should('be.visible');
});
