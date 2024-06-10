# Custom DNS Service

This project provides a custom DNS service designed to handle multiple nameservers with specific caching strategies for each. It's particularly useful for managing DNS queries across different geographic locations with optimized responses based on the originating request's characteristics.

## Overview

The Custom DNS Service routes DNS queries through specific paths based on the request origin and the requested domain. It utilizes a series of predefined rules to either use a general cache or a specialized cache for Chinese domains (referred to as `cache_cn`). The architecture allows for dynamic routing of DNS requests to optimize resolution speed and accuracy.

## Architecture

![DNS Routing Diagram](img/1.png)

### Components

- **Pre-routing (30003, 30004, 30006, 30007):** These are entry points for DNS queries, which decide the routing path based on the SOCKS settings.
- **Nameservers (30003, 30004, 30006, 30007):** These handle DNS requests by communicating with specified upstream nameservers.
- **Caches:**
  - **General Cache:** Used for caching DNS queries to reduce response time.
  - **cache_cn:** A specialized cache used exclusively for Chinese domain names to improve efficiency and response accuracy within China.
- **Upstream Nameservers (e.g., 223.5.5.5, 119.29.29.29):** These are external DNS services that resolve DNS queries which are not cached.

## Configuration

### DNS Service Settings

DNS settings can be adjusted through configuration files where each nameserver and its associated cache are defined. Example configuration:

```ini
[nameserver_30003]
address = 223.5.5.5
cache = cache_30003

[nameserver_30004]
address = 119.29.29.29
cache = cache_30004

[nameserver_30006]
address = 223.5.5.5
cache = cache_cn

[nameserver_30007]
address = 119.29.29.29
cache = cache_30007
