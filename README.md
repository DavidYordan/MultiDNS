# MultiDNS

MultiDNS is a DNS split-routing project for OpenWrt, implemented in Go. It routes DNS requests from different LAN segments to corresponding ports and handles DNS resolution based on specified configurations.

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
│   ├── upstream
│   │   └── upstream.go     # Upstream DNS query logic
│   └── utils
│       └── utils.go        # Utility functions
│
├── pkg
│   ├── cache
│   │   └── dns_cache.go    # Cache handling logic
│
├── cmd
│   └── multidns
│       └── main.go         # Main entry point, responsible for starting the service
│
├── .gitignore
├── go.mod
└── go.sum
```

## Configuration

The configuration file (`multidns.yaml`) is structured as follows:

```yaml
servers:
  - segment: 3
    stream_split: false

  - segment: 4
    stream_split: true

  - segment: 5
    stream_split: true

capacity: 1024

upstream_cn:
    address: ["223.5.5.5", "119.29.29.29"]

upstream_non_cn:
    address: ["1.1.1.1", "8.8.8.8"]

cn_domain_file: "/etc/multidns/cn_site.list"
```

### Fields

- `servers`: List of server configurations, each with a segment and a stream split option.
- `capacity`: The maximum capacity of the DNS cache in MB.
- `upstream_cn`: List of upstream DNS servers for CN domains.
- `upstream_non_cn`: List of upstream DNS servers for non-CN domains.
- `cn_domain_file`: Path to the file containing the list of CN domains.

## Usage

### Building the Project

To build the project, run:

```sh
go build -o multidns ./cmd/multidns
```

### Running the Server

To start the server, execute the built binary:

```sh
./multidns
```

Make sure the `multidns.yaml` configuration file is in the correct path (`/etc/multidns/multidns.yaml`).

## Functionality

1. **DNS Request Handling**: Routes DNS requests from different LAN segments to corresponding ports using nftables rules.
2. **Cache Management**: Utilizes a DNS cache to store responses and reduce latency for repeated queries.
3. **Upstream Query**: Resolves DNS queries using specified upstream DNS servers based on domain matching.

## Contributing

Feel free to contribute to the project by submitting issues or pull requests.

## License

This project is licensed under the MIT License. See the LICENSE file for details.
