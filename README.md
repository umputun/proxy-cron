[![Build Status](https://github.com/umputun/proxy-cron/workflows/build/badge.svg)](https://github.com/umputun/proxy-cron/actions) [![Coverage Status](https://coveralls.io/repos/github/umputun/proxy-cron/badge.svg?branch=master)](https://coveralls.io/github/umputun/proxy-cron?branch=master)


`proxy-cron` is a simple HTTP proxy server designed to handle requests based on crontab-like scheduling. It allows requests to be proxied during specified times and serves cached responses when requests are made outside the allowed schedule. This is particularly useful for managing systems that aren't running 24/7, to avoid unnecessary alerts or checks during their downtime.

## why it's needed

Imagine you have a service that is not running 24/7, but you still want to monitor it. You can't just use a regular monitoring system because it will alert you every time the service is down. You can't use a cron job to check the service because it will only run at specific times. You need a way to proxy requests to the service only when it's running, and serve cached responses when it's not.

With `proxy-cron`, you can define a schedule directly in the url for when requests should be proxied to the service. During the allowed times, requests are proxied as usual. When requests are made outside of the allowed times, `proxy-cron` serves the last cached response. This way, you can monitor the service without being alerted every time it's down.

## how it works

`proxy-cron` runs as a simple HTTP server handling `GET` request only. The request must contain two query parameters:
- `endpoint`: The actual endpoint URL you want to proxy.
- `crontab`: The crontab schedule expression defining the allowed times for proxying requests.

When a request is made to it, it checks the `crontab` query parameter to see if the request is allowed at the current time.
If the request is allowed, `proxy-cron` proxies the request to the specified endpoint, caches the response, and serves it to the client.
If the request is not allowed, `proxy-cron` serves the last cached response.

## application options

```
      --port=            port to listen on (default: 8080) [$PORT]
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
