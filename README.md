# Go APT Cacher

GoAPTCacher is a pull-through apt cacher similar to apt-cacher-ng and squid with local file cache. It is designed to be used in isolated environments and CI/CD pipelines to speed up package downloads and reduce the load on the package mirrors.

## Features

- Pull-through apt cacher - Only cache packages that were requested by clients
- Local file cache - Cache packages in a local directory to avoid re-downloading, also allows for pre-seeding the cache
- HTTP and HTTPS support - Serve packages over HTTP and HTTPS
- Support for on-the-fly certificate generation - Generate a self-signed certificate for HTTPS on the fly, uses intermediate CA for signing
- HTTPS-passthrough - Serve packages over HTTPS using the original package mirror's certificate
- Support for multiple package mirrors - Provide a list of allowed package repositories which clients can use
- Webinterface - View cache statistics and configuration details
- Proxy support - Configure GoAPTCacher as APT proxy server only for package downloads
- SRV record support - Automatically discover the GoAPTCacher server using DNS SRV records
- Rewrite support - Rewrite URLs to use a different package mirror, e.g. us.archive.ubuntu.com -> de.archive.ubuntu.com

## Installation

## What this program does



## What this progran not does



## Why this package was made?

I was dissatisfied with the performance and quality of the apt-cacher-ng package. As under [Ubuntu Bug Tracker](https://bugs.launchpad.net/ubuntu/+source/apt-cacher-ng) and [Debian Bug report logs](https://bugs.debian.org/cgi-bin/pkgreport.cgi?pkg=apt-cacher-ng) apt-cacher-ng has some unresolved issues. Some of these have also affected me and my friends over the years and have repeatedly hindered my home lab and my company and have repeatedly disrupted normal system operation and CI builds.

That's why I created an alternative with this program that is more in line with my wishes and requirements.
