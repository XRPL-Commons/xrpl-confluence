---
layout: home

hero:
  name: XRPL Confluence
  text: Mixed-network interop testing for the XRP Ledger
  tagline: A Kurtosis harness that runs rippled and go-xrpl side by side, then fuzzes, soaks and breaks them to prove they agree ledger-for-ledger.
  image:
    src: /commons_ligth_logo.png
    alt: XRPL Commons
  actions:
    - theme: brand
      text: Get Started
      link: /quickstart
    - theme: alt
      text: Overview
      link: /overview
    - theme: alt
      text: GitHub
      link: https://github.com/XRPL-Commons/xrpl-confluence

features:
  - icon: 🕸️
    title: Mixed networks
    details: Spin up any ratio of rippled + go-xrpl validators inside one private UNL and watch them reach consensus together.
  - icon: 🔬
    title: Differential oracle
    details: Every closed ledger is cross-checked across implementations — divergent account_hash / ledger_hash is captured as a finding with the suspect transactions labelled.
  - icon: 🔥
    title: Soak, fuzz & chaos
    details: Drive randomized and mutated transaction streams for hours, inject latency, restarts and partitions, and shrink any failure to a minimal reproducer.
  - icon: 📊
    title: Live dashboard
    details: A real-time view of every node's ledger, peers and logs, plus optional Prometheus + Grafana for long soak runs.
---

## What is XRPL Confluence?

**XRPL Confluence** is a [Kurtosis](https://www.kurtosis.com/)-based testing harness for verifying
interoperability between XRP Ledger node implementations. It boots a mixed network of
[rippled](https://github.com/XRPLF/rippled) (the C++ reference implementation) and
[go-xrpl](https://github.com/LeJamon/goXRPLd) (a native Go implementation) as first-class validators
in a single private network, then runs a sidecar that submits transactions and continuously checks
that every node agrees on the resulting ledger state.

It is the **differential-testing layer** of the XRPL interop stack: where fixtures pin individual
transaction outcomes, Confluence pins whole-network behaviour — propagation, sync, consensus, and
long-horizon stability under load and fault injection.

## Where to go next

- **[Overview](/overview)** — the architecture and the problem it solves.
- **[Quickstart](/quickstart)** — boot your first mixed network in a few commands.
- **[Test Suites](/test-suites)** — what each suite asserts.
- **[CLI & Scenarios](/cli)** — the `confluence` CLI and declarative Scenario YAML.
