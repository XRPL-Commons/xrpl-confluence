"""Kurtosis service definition for an optional Grafana sidecar.

Started by `enable_observability=true` in the soak suite. Comes with a
pre-provisioned Prometheus datasource pointing at the prometheus service.
Anonymous viewer access is enabled so operators can open the URL without
logging in (this is a private testnet, no PII).
"""


def launch(plan, prometheus_service_name = "prometheus"):
    """Launch a Grafana sidecar with Prometheus pre-provisioned.

    Args:
        plan: Kurtosis plan object.
        prometheus_service_name: Service name of the Prometheus sidecar.

    Returns:
        Grafana service reference.
    """
    provisioning = plan.upload_files(
        src = "../../dashboard/grafana-provisioning",
        name = "grafana-provisioning",
    )

    return plan.add_service(
        name = "grafana",
        config = ServiceConfig(
            image = "grafana/grafana:latest",
            ports = {
                "http": PortSpec(
                    number = 3000,
                    transport_protocol = "TCP",
                    application_protocol = "http",
                ),
            },
            files = {
                "/etc/grafana/provisioning": provisioning,
            },
            env_vars = {
                "GF_AUTH_ANONYMOUS_ENABLED": "true",
                "GF_AUTH_ANONYMOUS_ORG_ROLE": "Viewer",
                "GF_SECURITY_ADMIN_PASSWORD": "admin",
            },
        ),
    )
