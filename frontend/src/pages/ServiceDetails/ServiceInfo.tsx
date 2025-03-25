import * as React from 'react';
import { connect } from 'react-redux';
import { kialiStyle } from 'styles/StyleUtils';
import { Grid, GridItem, Stack, StackItem } from '@patternfly/react-core';
import { ServiceDescription } from './ServiceDescription';
import { ServiceId, ServiceDetailsInfo } from '../../types/ServiceInfo';
import {
  DestinationRuleC,
  filterAutogeneratedGateways,
  Gateway,
  getGatewaysAsList,
  getK8sGatewaysAsList,
  K8sGateway,
  ObjectValidation,
  PeerAuthentication,
  Validations
} from '../../types/IstioObjects';
import { RenderComponentScroll } from '../../components/Nav/Page';
import { PromisesRegistry } from 'utils/CancelablePromises';
import { DurationInSeconds } from 'types/Common';
import { GraphDataSource } from 'services/GraphDataSource';
import {
  drToIstioItems,
  vsToIstioItems,
  gwToIstioItems,
  seToIstioItems,
  k8sHTTPRouteToIstioItems,
  k8sGRPCRouteToIstioItems,
  validationKey,
  k8sGwToIstioItems
} from '../../types/IstioConfigList';
import { canCreate, canUpdate } from '../../types/Permissions';
import { KialiAppState } from '../../store/Store';
import { durationSelector } from '../../store/Selectors';
import { ServiceNetwork } from './ServiceNetwork';
import { IstioConfigCard } from '../../components/IstioConfigCard/IstioConfigCard';
import { ServiceWizard } from '../../components/IstioWizards/ServiceWizard';
import { ConfirmDeleteTrafficRoutingModal } from '../../components/IstioWizards/ConfirmDeleteTrafficRoutingModal';
import { WizardAction, WizardMode } from '../../components/IstioWizards/WizardActions';
import { deleteServiceTrafficRouting } from '../../services/Api';
import * as AlertUtils from '../../utils/AlertUtils';
import { triggerRefresh } from '../../hooks/refresh';
import { serverConfig } from 'config';
import { MiniGraphCard } from 'pages/Graph/MiniGraphCard';

type ReduxProps = {
  duration: DurationInSeconds;
  istioAPIEnabled: boolean;
};

interface Props extends ServiceId, ReduxProps {
  cluster?: string;
  duration: DurationInSeconds;
  gateways: Gateway[];
  istioAPIEnabled: boolean;
  k8sGateways: K8sGateway[];
  peerAuthentications: PeerAuthentication[];
  serviceDetails?: ServiceDetailsInfo;
  validations: Validations;
}

type ServiceInfoState = {
  showConfirmDeleteTrafficRouting: boolean;
  // Wizards related
  showWizard: boolean;
  tabHeight?: number;
  updateMode: boolean;
  wizardType: string;
};

const fullHeightStyle = kialiStyle({
  height: '100%'
});

class ServiceInfoComponent extends React.Component<Props, ServiceInfoState> {
  private promises = new PromisesRegistry();
  private graphDataSource = new GraphDataSource();

  constructor(props: Props) {
    super(props);
    this.state = {
      tabHeight: 300,
      showWizard: false,
      wizardType: '',
      updateMode: false,
      showConfirmDeleteTrafficRouting: false
    };
  }

  componentDidMount(): void {
    this.fetchBackend();
  }

  componentDidUpdate(prev: Props): void {
    if (prev.duration !== this.props.duration || prev.serviceDetails !== this.props.serviceDetails) {
      this.fetchBackend();
    }
  }

  private fetchBackend = (): void => {
    if (!this.props.serviceDetails) {
      return;
    }

    this.promises.cancelAll();
    this.graphDataSource.fetchForService(
      this.props.duration,
      this.props.namespace,
      this.props.service,
      this.props.cluster
    );
  };

  private getServiceValidation(): ObjectValidation | undefined {
    if (this.props.validations && this.props.validations.service && this.props.serviceDetails) {
      return this.props.validations.service[
        validationKey(this.props.serviceDetails.service.name, this.props.namespace)
      ];
    }

    return undefined;
  }

  private handleWizardClose = (changed: boolean): void => {
    this.setState({
      showWizard: false
    });

    if (changed) {
      triggerRefresh();
    }
  };

  private handleConfirmDeleteServiceTrafficRouting = (): void => {
    this.setState({
      showConfirmDeleteTrafficRouting: false
    });

    deleteServiceTrafficRouting(this.props.serviceDetails!)
      .then(_results => {
        AlertUtils.addSuccess(`Istio Config deleted for ${this.props.serviceDetails?.service.name} service.`);

        triggerRefresh();
      })
      .catch(error => {
        AlertUtils.addError('Could not delete Istio config objects.', error);
      });
  };

  private handleDeleteTrafficRouting = (_key: string): void => {
    this.setState({ showConfirmDeleteTrafficRouting: true });
  };

  private handleLaunchWizard = (action: WizardAction, mode: WizardMode): void => {
    this.setState({
      showWizard: true,
      wizardType: action,
      updateMode: mode === 'update'
    });
  };

  render(): React.ReactNode {
    const vsIstioConfigItems = this.props.serviceDetails?.virtualServices
      ? vsToIstioItems(
          this.props.serviceDetails.virtualServices,
          this.props.serviceDetails.validations,
          this.props.cluster
        )
      : [];

    const drIstioConfigItems = this.props.serviceDetails?.destinationRules
      ? drToIstioItems(
          this.props.serviceDetails.destinationRules,
          this.props.serviceDetails.validations,
          this.props.cluster
        )
      : [];

    const gwIstioConfigItems =
      this.props?.gateways && this.props.serviceDetails?.virtualServices
        ? gwToIstioItems(
            this.props?.gateways,
            this.props.serviceDetails.virtualServices,
            this.props.serviceDetails.validations,
            this.props.cluster
          )
        : [];

    const k8sGwIstioConfigItems =
      this.props?.k8sGateways && (this.props.serviceDetails?.k8sHTTPRoutes || this.props.serviceDetails?.k8sGRPCRoutes)
        ? k8sGwToIstioItems(
            this.props?.k8sGateways,
            this.props.serviceDetails.k8sHTTPRoutes,
            this.props.serviceDetails.k8sGRPCRoutes,
            this.props.serviceDetails.validations,
            this.props.cluster,
            this.props.serviceDetails?.service?.labels
              ? this.props.serviceDetails.service.labels[serverConfig.istioLabels.ambientWaypointGatewayLabel]
              : ''
          )
        : [];

    const seIstioConfigItems = this.props.serviceDetails?.serviceEntries
      ? seToIstioItems(
          this.props.serviceDetails.serviceEntries,
          this.props.serviceDetails.validations,
          this.props.cluster
        )
      : [];

    const k8sHTTPRouteIstioConfigItems = this.props.serviceDetails?.k8sHTTPRoutes
      ? k8sHTTPRouteToIstioItems(
          this.props.serviceDetails.k8sHTTPRoutes,
          this.props.serviceDetails.validations,
          this.props.cluster
        )
      : [];

    const k8sGRPCRouteIstioConfigItems = this.props.serviceDetails?.k8sGRPCRoutes
      ? k8sGRPCRouteToIstioItems(
          this.props.serviceDetails.k8sGRPCRoutes,
          this.props.serviceDetails.validations,
          this.props.cluster
        )
      : [];

    const istioConfigItems = seIstioConfigItems.concat(
      gwIstioConfigItems.concat(
        k8sGwIstioConfigItems.concat(
          vsIstioConfigItems.concat(
            drIstioConfigItems.concat(k8sHTTPRouteIstioConfigItems.concat(k8sGRPCRouteIstioConfigItems))
          )
        )
      )
    );

    // RenderComponentScroll handles height to provide an inner scroll combined with tabs
    // This height needs to be propagated to minigraph to proper resize in height
    // Graph resizes correctly on width
    const miniGraphSpan = 8;

    return (
      <>
        <RenderComponentScroll onResize={height => this.setState({ tabHeight: height })}>
          <Grid hasGutter={true} className={fullHeightStyle}>
            <GridItem span={4}>
              <Stack hasGutter={true}>
                <StackItem>
                  <ServiceDescription namespace={this.props.namespace} serviceDetails={this.props.serviceDetails} />
                </StackItem>

                {this.props.serviceDetails && (
                  <ServiceNetwork
                    serviceDetails={this.props.serviceDetails}
                    gateways={this.props.gateways}
                    validations={this.getServiceValidation()}
                  />
                )}

                <StackItem style={{ paddingBottom: '20px' }}>
                  <IstioConfigCard name={this.props.service} items={istioConfigItems} />
                </StackItem>
              </Stack>
            </GridItem>

            <GridItem span={miniGraphSpan}>
              <MiniGraphCard
                dataSource={this.graphDataSource}
                onDeleteTrafficRouting={this.handleDeleteTrafficRouting}
                onLaunchWizard={this.handleLaunchWizard}
                serviceDetails={this.props.serviceDetails}
              />
            </GridItem>
          </Grid>
        </RenderComponentScroll>

        <ServiceWizard
          show={this.state.showWizard}
          type={this.state.wizardType}
          update={this.state.updateMode}
          namespace={this.props.namespace}
          cluster={this.props.cluster}
          serviceName={this.props.serviceDetails?.service?.name ?? ''}
          workloads={this.props.serviceDetails?.workloads ?? []}
          subServices={this.props.serviceDetails?.subServices ?? []}
          createOrUpdate={
            canCreate(this.props.serviceDetails?.istioPermissions) ||
            canUpdate(this.props.serviceDetails?.istioPermissions)
          }
          virtualServices={this.props.serviceDetails?.virtualServices ?? []}
          destinationRules={this.props.serviceDetails?.destinationRules ?? []}
          gateways={getGatewaysAsList(filterAutogeneratedGateways(this.props.gateways))}
          k8sGateways={getK8sGatewaysAsList(this.props.k8sGateways)}
          k8sGRPCRoutes={this.props.serviceDetails?.k8sGRPCRoutes ?? []}
          k8sHTTPRoutes={this.props.serviceDetails?.k8sHTTPRoutes ?? []}
          peerAuthentications={this.props.peerAuthentications}
          tlsStatus={this.props.serviceDetails?.namespaceMTLS}
          onClose={this.handleWizardClose}
          istioAPIEnabled={this.props.istioAPIEnabled}
        />

        {this.state.showConfirmDeleteTrafficRouting && (
          <ConfirmDeleteTrafficRoutingModal
            destinationRules={DestinationRuleC.fromDrArray(this.props.serviceDetails!.destinationRules)}
            virtualServices={this.props.serviceDetails!.virtualServices}
            k8sHTTPRoutes={this.props.serviceDetails!.k8sHTTPRoutes}
            k8sGRPCRoutes={this.props.serviceDetails!.k8sGRPCRoutes}
            isOpen={true}
            onCancel={() => this.setState({ showConfirmDeleteTrafficRouting: false })}
            onConfirm={this.handleConfirmDeleteServiceTrafficRouting}
          />
        )}
      </>
    );
  }
}

const mapStateToProps = (state: KialiAppState): ReduxProps => ({
  duration: durationSelector(state),
  istioAPIEnabled: state.statusState.istioEnvironment.istioAPIEnabled
});

export const ServiceInfo = connect(mapStateToProps)(ServiceInfoComponent);
