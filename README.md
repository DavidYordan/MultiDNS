# MultiDNS

MultiDNS is a highly customizable DNS service developed in Go. It processes DNS query requests on multiple ports according to different configuration files and rules, and applies different caching strategies based on a specific domain list (cn_site.list). This service is particularly suitable for scenarios that require dynamically selecting the optimal resolution path based on the origin or content of the request.

## Project Structure

```
/multidns
│
├── internal
│   ├── config
│   │   └── config.go       # Configuration loading and handling
│   ├── dns
│   │   ├── server.go       # DNS server startup logic
│   │   ├── request.go      # Request handling logic
│   │   ├── socket.go       # Socket creation logic
│   │   └── upstream.go     # Upstream DNS query logic
│   └── utils
│       └── utils.go        # Utility functions
│
├── pkg
│   ├── cache
│   │   └── cache.go        # Cache handling logic
│
├── cmd
│   └── multidns
│       └── main.go         # Main entry point, responsible for starting the service
│
├── .gitignore
├── go.mod
└── go.sum
```

## Features

- **Multi-port Listening**: Capable of listening for DNS queries on multiple ports simultaneously, providing services for different types of requests.
- **Conditional Caching Logic**: Special caching logic for Chinese domains, while other domains follow different caching strategies based on configuration.
- **Highly Configurable**: Control listening ports and resolution strategies through external configuration files, making the service flexible and easy to adjust.
- **Command-line Management**: Provides scripts to support command-line startup, stop, and restart of the service, making it easy to maintain and manage.

## Installation and Running

### Prerequisites

- Go 1.16 or higher
- OpenWRT system
- Configured nftables and related TPROXY rules

### Configuration Files

Create `multidns.yaml` and `cn_site.list` files in the `/etc/multidns` directory.

#### `multidns.yaml`

```yaml
servers:
  - id: "30003"
    stream_split: false
    cache_capacity: 50000

  - id: "30004"
    stream_split: true
    cache_capacity: 50000

  - id: "30005"
    stream_split: true
    cache_capacity: 50000

cache_cn:
  capacity: 100000

upstream_cn:
    address: ["223.5.5.5", "119.29.29.29"]
```

#### `cn_site.list`

This file contains the Chinese domains that require special handling, one domain per line.

### Build and Run

1. Clone the project code:
   ```sh
   git clone https://github.com/yourusername/multidns.git
   cd multidns
   ```

2. Build the project:
   ```sh
   go build -o multidns ./cmd/multidns
   ```

3. Run the project:
   ```sh
   ./multidns
   ```

### Routing and Firewall Configuration

Configure routing and firewall rules on the OpenWRT system to ensure MultiDNS correctly processes and returns DNS requests and responses.

```sh
ip rule add fwmark 1 table 100
ip route add local default dev lo table 100
```

Use nftables to configure TPROXY rules:

```sh
table ip xray {
    map saddr_to_tproxy {
        type ipv4_addr : verdict
        flags interval
        elements = { 192.168.3.0/24 : goto prerouting_30003, 192.168.4.0/24 : goto prerouting_30004,
                     192.168.5.0/24 : goto prerouting_30005 }
    }

    chain prerouting {
        type filter hook prerouting priority mangle; policy accept;
        ip saddr vmap @saddr_to_tproxy
    }

    chain prerouting_30003 {
        meta nftrace set 1
        udp dport 53 tproxy to :32003 meta mark set 0x00000001 accept
        tcp dport != 0 tproxy to :30003 meta mark set 0x00000001 accept
        udp dport != 0 tproxy to :30003 meta mark set 0x00000001 accept
    }

    chain prerouting_30004 {
        udp dport 53 tproxy to :32004 meta mark set 0x00000001 accept
        tcp dport != 0 tproxy to :30004 meta mark set 0x00000001 accept
        udp dport != 0 tproxy to :30004 meta mark set 0x00000001 accept
    }

    chain prerouting_30005 {
        udp dport 53 tproxy to :32005 meta mark set 0x00000001 accept
        tcp dport != 0 tproxy to :30005 meta mark set 0x00000001 accept
        udp dport != 0 tproxy to :30005 meta mark set 0x00000001 accept
    }
}
```

## Contributing

Feel free to submit issues and pull requests! Please make sure to run all tests and adhere to the project's code style before submitting code.

## License

This project is licensed under the MIT License. See the LICENSE file for details.
