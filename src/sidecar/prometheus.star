"""Kurtosis service definition for an optional Prometheus sidecar.

Started by `enable_observability=true` in the soak suite. Scrapes the fuzz
sidecar's /metrics endpoint at a configurable interval. Useful for week-long
soak runs where the dashboard's in-memory KPIs aren't enough — Grafana plus
this Prometheus surface histograms over time.
"""


def launch(plan, fuzz_service_name = "fuzz-soak", scrape_interval_s = 5):
    """Launch a Prometheus sidecar scraping the fuzz service.

    Args:
        plan: Kurtosis plan object.
        fuzz_service_name: Service name of the fuzz sidecar (default "fuzz-soak").
        scrape_interval_s: Scrape interval in seconds.

    Returns:
        Prometheus service reference.
    """
    cfg = """\
global:
  scrape_interval: {s}s
scrape_configs:
  - job_name: fuzz
    static_configs:
      - targets: ["{f}:8081"]
""".format(s = scrape_interval_s, f = fuzz_service_name)

    cfg_artifact = plan.render_templates(
        name = "prometheus-config",
        config = {"prometheus.yml": struct(template = cfg, data = {})},
    )

    return plan.add_service(
        name = "prometheus",
        config = ServiceConfig(
            image = "prom/prometheus:latest",
            ports = {
                "http": PortSpec(
                    number = 9090,
                    transport_protocol = "TCP",
                    application_protocol = "http",
                ),
            },
            files = {"/etc/prometheus": cfg_artifact},
            cmd = ["--config.file=/etc/prometheus/prometheus.yml"],
        ),
    )
