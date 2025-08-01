@external-kiali
# Comprehensive tests for external kiali deployment

Feature: External Kiali functionality

  These tests verify that Kiali running in a management cluster can properly 
  observe and interact with applications and infrastructure in a separate mesh cluster.
  In this setup, Kiali is deployed externally from the service mesh it monitors.

  Background:
    Given user is at administrator perspective

  Scenario: Graph shows correct mesh cluster infrastructure
    When user is at the "graph" page
    And user graphs "istio-system" namespaces
    Then user sees the "istio-system" namespace
    And user sees Istio control plane components in the "mesh" cluster
    And no control plane components should be visible in the "mgmt" cluster graph

  Scenario: Istio config visible but read-only from mesh cluster
    When user is at the "istio" page
    And user selects the "istio-system" namespace
    Then user sees Istio configurations from the "mesh" cluster
    And Istio config objects should be read-only
    And user should not see create/edit/delete options for mesh cluster configs

  Scenario: Istio config editor shows read-only view for mesh cluster objects
    When user is at the "istio" page
    And user selects the "istio-system" namespace
    And user clicks on any Istio configuration from the "mesh" cluster
    Then user sees the Istio configuration in read-only mode
    And edit and delete buttons should be disabled

  Scenario: Mesh topology shows correct infrastructure separation
    When user is at the "mesh" page
    Then user sees Kiali running in the "mgmt" cluster
    And user sees Istio control plane running in the "mesh" cluster
    And user sees external service connections between clusters
    And user sees Prometheus running in the "mesh" cluster
    And user sees the correct cluster separation in mesh topology

  Scenario: Mesh page shows external service connections
    When user is at the "mesh" page
    Then user sees connections from Kiali to external Prometheus
    And user sees connections from Kiali to external Grafana if enabled
    And user sees connections from Kiali to external tracing if enabled
    And external service nodes should be clearly marked

  Scenario: Cluster badge displays correctly in overview
    When user is at the "overview" page
    Then user sees the "mgmt" cluster badge for Kiali namespace
    And user sees the "mesh" cluster badge for istio-system namespace
    And cluster badges should distinguish between mgmt and mesh clusters

  Scenario: Health indicators work across clusters
    When user is at the "overview" page
    Then user sees health indicators for namespaces in the "mesh" cluster
    And health status should reflect actual workload health from mesh cluster
    And health calculations should work with external metrics

  Scenario: Traffic metrics display from external cluster
    When user is at the "overview" page
    And user selects "Last 10m" time range
    Then user sees traffic metrics for namespaces in the "mesh" cluster
    And traffic data should come from external Prometheus
    And inbound/outbound traffic should be accurately displayed

  Scenario: Namespace filtering works with external cluster data
    When user is at the "overview" page
    And user filters "istio-system" namespace
    Then user sees only the "istio-system" namespace from the "mesh" cluster
    And filtering should work correctly with external data sources

  Scenario: Validations work with external Istio API
    When user is at the "istio" page
    And user selects the "istio-system" namespace
    Then user sees validation status for Istio configurations
    And validations should work with external Istio API access
    And validation errors should be displayed correctly

  Scenario: Search functionality works across clusters
    When user uses the global search functionality
    And user searches for "istiod"
    Then search results should include objects from the "mesh" cluster
    And search should work with external cluster data

  Scenario: Istio panels for mgmt and mesh clusters should be visible
    When user is at the "overview" page
    Then user sees the "istio-system" namespace card in cluster "mgmt"
    And user sees the "istio-system" namespace card in cluster "mesh"
    And user sees the "Control plane" label in the "mesh" "istio-system" namespace card
    And user does not see the "Control plane" label in the "mgmt" "istio-system" namespace card
    And Istio config should not be available for the "mgmt" "istio-system" 