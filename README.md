# Selenoid
[![ci](https://github.com/i-shulgan/selenoid/actions/workflows/ci.yml/badge.svg)](https://github.com/i-shulgan/selenoid/actions/workflows/ci.yml)

Selenoid is a powerful implementation of [Selenium](http://github.com/SeleniumHQ/selenium) hub using [Docker](https://docker.com/) containers to launch browsers.
![Selenoid Animation](docs/img/selenoid-animation.gif)

This is a maintained fork of [aerokube/selenoid](https://github.com/aerokube/selenoid), which is archived since 1.11.3. It pairs with [i-shulgan/ggr](https://github.com/i-shulgan/ggr) and with current Chrome images from [i-shulgan/images](https://github.com/i-shulgan/images).

## Differences from upstream

Fixes:

* **File upload** (`-enable-file-upload`): a zip entry like `../../etc/cron.d/x` was written outside the session directory (zip slip), and the `/file` endpoint took the target directory from a request header, so any client could write files anywhere Selenoid can. Uploads are now confined to the directory of a running session; a shorter upload no longer keeps the tail of a previous file.
* **New session retries** (`-retry-count` > 1) with drivers under `/wd/hub` went to `/wd/hub/wd/hub/session` and returned 404.
* **Unexpected driver responses**: a non-string `sessionId` panicked the handler and leaked a queue slot for good (lowering `-limit`); a W3C reply without `capabilities` was passed to the client but the session was not registered; non-JSON replies (HTML error pages) gave an empty response, now a `session not created` error with the status and the beginning of the body.
* **Race in `/devtools/`, `/download/`, `/clipboard/`**: they re-armed the session timeout without the session lock, concurrent requests could close the timer channel twice and panic.
* **Container log config** (`-log-conf`): the shared configuration map was modified for every session, so the first test name stuck as the tag of all containers and parallel starts could crash Selenoid with `concurrent map writes`.
* **Version by prefix** was chosen in random map order (`12` could pick `120.0`); now an exact match, then a whole version component, then the highest version.
* A browser container was left running when connecting an `additionalNetworks` network failed; a port without host bindings panicked the handler; the readiness check could hang forever on a service that accepts connections but never answers.
* A non-positive `sessionTimeout` killed the session at once; video and log directories were created without the execute bit; signals could be dropped.

Build and delivery:

* Go 1.27.2; Docker client moved from `github.com/docker/docker` to `github.com/moby/moby/client` (govulncheck reported GO-2026-4887 for the old module); the API version is negotiated with the daemon, Docker API 1.40 (Docker 19.03) or newer is required. S3 upload moved to aws-sdk-go-v2 (v1 is out of support).
* Multi-stage `Dockerfile` building a static binary; `linux/amd64` and `linux/arm64` images in `ghcr.io/i-shulgan/selenoid`. The image runs as root because it needs the Docker socket. Build with `--build-arg BUILD_TAGS="s3 metadata"` for S3 upload and session metadata.
* CI: `go vet`, `staticcheck`, `go test -race` and `govulncheck` with and without the `s3 metadata` tags; semantic versions with release-please.

## Running

```bash
docker run -d --name selenoid -p 4444:4444 \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v /etc/selenoid:/etc/selenoid:ro \
    -v /opt/selenoid/video:/opt/selenoid/video \
    ghcr.io/i-shulgan/selenoid:latest -limit 10
```

`/etc/selenoid/browsers.json` with the current Chrome from [i-shulgan/images](https://github.com/i-shulgan/images):

```json
{"chrome": {"default": "141.0", "versions": {"141.0": {"image": "ghcr.io/i-shulgan/chrome:141.0", "port": "4444", "path": "/", "shmSize": 2147483648}}}}
```

Browser images must be pulled beforehand. Image tags: `latest` and `sha-<short-sha>` for commits to `master`, `X.Y.Z`, `X.Y`, `X` for releases.

## Installation without Docker

Every release also has static Linux binaries (built without CGO, they run on Debian 13 and other distributions) and `SHA256SUMS`:

| File | Content |
|------|---------|
| `selenoid_linux_amd64`, `selenoid_linux_arm64` | default build, same names as upstream release assets |
| `selenoid_linux_amd64_s3_metadata`, `selenoid_linux_arm64_s3_metadata` | built with the `s3 metadata` tags: S3 upload and session metadata |
| `SHA256SUMS` | SHA-256 of all files of the release |

```bash
VERSION=vX.Y.Z
curl -fsSLO https://github.com/i-shulgan/selenoid/releases/download/$VERSION/selenoid_linux_amd64 \
  && curl -fsSLO https://github.com/i-shulgan/selenoid/releases/download/$VERSION/SHA256SUMS \
  && sha256sum -c --ignore-missing SHA256SUMS
sudo install -m 0755 selenoid_linux_amd64 /usr/bin/selenoid
selenoid -version
```

Ansible checks the binary against the same file:

```yaml
- name: Install selenoid
  ansible.builtin.get_url:
    url: "https://github.com/i-shulgan/selenoid/releases/download/{{ selenoid_version }}/selenoid_linux_amd64"
    checksum: "sha256:https://github.com/i-shulgan/selenoid/releases/download/{{ selenoid_version }}/SHA256SUMS"
    dest: /usr/bin/selenoid
    mode: "0755"
```

Selenoid needs access to the Docker socket: run it as root or as a user in the `docker` group. [`packaging/selenoid.service`](packaging/selenoid.service) is a systemd unit for a `selenoid` system user (`useradd --system --no-create-home --shell /usr/sbin/nologin --groups docker selenoid`); it reads `/etc/selenoid/browsers.json`, writes videos to `/var/lib/selenoid/video` and takes extra flags from `SELENOID_ARGS` in `/etc/default/selenoid`.

A deb package (`selenoid_X.Y.Z_amd64.deb`, `selenoid_X.Y.Z_arm64.deb`: the binary, the systemd unit, the `selenoid` user, `Depends: docker-ce | docker.io`, the service is not started) is described in [`nfpm.yaml`](nfpm.yaml) but not published yet. To add it to releases and `SHA256SUMS`, set the repository variable `BUILD_DEB` to `true` (**Settings → Secrets and variables → Actions → Variables**).

## Releases and versioning

Versions follow [Semantic Versioning](https://semver.org), continue from upstream `1.11.3` and are derived from [Conventional Commits](https://www.conventionalcommits.org) by [release-please](https://github.com/googleapis/release-please): `fix:` bumps the patch version, `feat:` the minor one, `!` or `BREAKING CHANGE:` the major one. After merges to `master` release-please opens a `chore(master): release X.Y.Z` pull request; merging it tags `vX.Y.Z`, publishes a GitHub release with the binaries and `SHA256SUMS` and pushes `ghcr.io/i-shulgan/selenoid:X.Y.Z`, `:X.Y` and `:X`. Pull request titles must follow Conventional Commits (the `pr-title` check), since squash merges use them as commit messages.

## Features

### One-command Installation
Start browser automation in minutes by downloading [Configuration Manager](https://github.com/aerokube/cm/releases) binary and running just **one command**:
```
$ ./cm selenoid start --vnc --tmpfs 128
```
**That's it!** You can now use Selenoid instead of Selenium server. Specify the following Selenium URL in tests:
```
http://localhost:4444/wd/hub
```

### Ready to use Browser Images
No need to manually install browsers or dive into WebDriver documentation. Available images:
![Browsers List](docs/img/browsers-list.gif)

New images are added right after official releases. You can create your custom images with browsers. 

### Live Browser Screen and Logs
New **[rich user interface]((https://github.com/aerokube/selenoid-ui))** showing browser screen and Selenium session logs:
![Selenoid UI](docs/img/selenoid-ui.png)

### Video Recording
* Any browser session can be saved to [H.264](https://en.wikipedia.org/wiki/H.264/MPEG-4_AVC) video ([example](https://www.youtube.com/watch?v=maB298oO5cI))
* An API to list, download and delete recorded video files

### Convenient Logging

* Any browser session logs are automatically saved to files - one per session
* An API to list, download and delete saved log files

### Lightweight and Lightning Fast
Suitable for personal usage and in big clusters:
* Consumes **10 times** less memory than Java-based Selenium server under the same load
* **Small 6 Mb binary** with no external dependencies (no need to install Java)
* **Browser consumption API** working out of the box
* Ability to send browser logs to **centralized log storage** (e.g. to the [ELK-stack](https://logz.io/learn/complete-guide-elk-stack/))
* Fully **isolated** and **reproducible** environment

### Detailed Documentation and Free Support
Maintained by a growing community:
* Detailed [documentation](http://aerokube.com/selenoid/latest/)
* Telegram [support channel](https://t.me/aerokube)
* Support by [email](mailto:support@aerokube.com)
* StackOverflow [tag](https://stackoverflow.com/questions/tagged/selenoid)
* YouTube [channel](https://www.youtube.com/channel/UC9HvE3FNfTvftzpvXi9c69g)

## Complete Guide

The upstream reference guide applies to this fork: http://aerokube.com/selenoid/latest/

## Selenoid in Kubernetes

Selenoid was initially created to be deployed on hardware servers or virtual machines and is not suitable for Kubernetes. Detailed motivation is described [here](https://aerokube.com/selenoid/latest/#_selenoid_in_kubernetes). If you still need running Selenium tests in Kubernetes, then take a look at [Moon](https://github.com/aerokube/moon/) - our dedicated solution for Kubernetes. 

## Known Users

[![JetBrains](docs/img/logo/jetbrains.png)](http://jetbrains.com/) [![Yandex](docs/img/logo/yandex.png)](https://yandex.com/company/) [![Sberbank Technology](docs/img/logo/sbertech.png)](http://sber-tech.com/) [![ThoughtWorks](docs/img/logo/thoughtworks.png)](https://thoughtworks.com/) [![VK.com](docs/img/logo/vk.png)](https://vk.com/) [![SuperJob](docs/img/logo/superjob.png)](http://superjob.ru/) [![PropellerAds](docs/img/logo/propellerads.png)](http://propellerads.com/) [![AlfaBank](docs/img/logo/alfabank.png)](https://alfabank.com/) [![3CX](docs/img/logo/3cx.png)](https://www.3cx.com/) [![IQ Option](docs/img/logo/iq_option.png)](https://iqoption.com/) [![Mail.Ru Group](docs/img/logo/mail_ru.png)](https://corp.mail.ru/en/) [![Newegg.Com](docs/img/logo/newegg.png)](https://newegg.com/) [![Badoo](docs/img/logo/badoo.png)](https://badoo.com/team/) [![BCS](docs/img/logo/bcs.png)](https://bcs.ru/) [![Quality Lab](docs/img/logo/quality-lab.png)](https://quality-lab.ru) [![AT Consulting](docs/img/logo/at-consulting.png)](https://www.at-consulting.ru/) [![Royal Caribbean International](docs/img/logo/royal-caribbean.png)](https://www.royalcaribbean.com/) [![Sixt](docs/img/logo/sixt.png)](https://sixt.com/) [![Testjar](docs/img/logo/testjar.png)](http://www.testjar.com/) [![Flipdish](docs/img/logo/flipdish.png)](https://www.flipdish.com/) [![RiAdvice](docs/img/logo/riadvice.png)](https://riadvice.tn/)

