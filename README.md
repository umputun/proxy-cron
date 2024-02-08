[![Build Status](https://github.com/umputun/proxy-cron/workflows/build/badge.svg)](https://github.com/umputun/proxy-cron/actions) [![Coverage Status](https://coveralls.io/repos/github/umputun/proxy-cron/badge.svg?branch=master)](https://coveralls.io/github/umputun/proxy-cron?branch=master)


Proxy-cron is a simple HTTP proxy server designed to handle requests based on crontab-like scheduling. It enables requests to be proxied during specified times and serves cached responses when requests are made outside the allowed schedule. This is particularly useful for managing systems that aren't operational 24/7, to prevent unnecessary alerts or checks during their downtime.

## Why it's needed

Consider a service that does not operate around the clock, yet requires monitoring. Utilizing a regular monitoring system is not feasible, as it would generate alerts every time the service is offline. Similarly, a cron job is not a solution, as it only executes at specific intervals. A method is needed to proxy requests to the service only when it's active and serve cached responses when it's not.

With proxy-cron, users can specify a schedule in the URL, determining when requests should be proxied to the service. During these permitted times, requests are proxied as usual. However, when requests occur outside of these times, proxy-cron provides the last cached response. This enables monitoring of the service without triggering alerts each time it becomes unavailable.

## How it works

Proxy-cron operates as a straightforward HTTP server, processing only `GET` requests. Each request must include two query parameters:
 
- `endpoint`: The actual endpoint URL you want to proxy.
- `crontab`: The crontab schedule expression defining the allowed times for proxying requests.

Upon receiving a request, proxy-cron evaluates the 'crontab' query parameter to determine if the current time falls within the allowed period. If so, proxy-cron proxies the request to the specified endpoint, caches the response, and forwards it to the client. If the request is made outside the permitted time, proxy-cron supplies the last cached response instead.

## Installation

proxy-cron is available as a Docker image and be loaded from the docker hub as `umputun/proxy-cron` and from the GitHub Container Registry as
`ghcr.io/umputun/proxy-cron`. Binary releases are also available on the [releases page](https://github.com/umputun/proxy-cron/releases).

For macOS users, proxy-cron can be installed using Homebrew: `brew install umputun/tap/proxy-cron`.

## Usage

To use the proxy, send HTTP requests to it with the following query parameters `endpoint` and `crontab`. For example:
```
curl "http://localhost:8080/?endpoint=http://example.com&crontab=* 8-16 * * 1-5"
```
note: the `crontab` parameter can be passed with `_` instead of spaces to avoid the need for URL encoding, 
i.e. `* 8-16 * * 1-5` becomes `*_8-16_*_* _1-5`.


## Application options

```
      --port=            port to listen on (default: 8080) [$PORT]
      --max-size=        max body size in bytes (default: 1048576) [$MAX_SIZE]
      --no-colors        disable colorized logging [$NO_COLORS]
      --dbg              debug mode [$DEBUG]

timeout:
      --timeout.connect= connect timeout (default: 10s) [$TIMEOUT_CONNECT]
      --timeout.read=    read timeout (default: 10s) [$TIMEOUT_READ]
      --timeout.write=   write timeout (default: 10s) [$TIMEOUT_WRITE]
      --timeout.idle=    idle timeout (default: 15s) [$TIMEOUT_IDLE]

Help Options:
  -h, --help             Show this help message

```
