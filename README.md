# s3t

S3 compatibility test suite as a single static binary.

`s3t` is a Go port of [ceph/s3-tests][upstream], the de-facto conformance suite for
software that exposes an S3-like API. It runs the same tests, under the same names,
against the same configuration file — without a Python runtime, a virtualenv, or `tox`.

> **Status: in development.** The harness runs and the first tests are ported; the bulk
> of the suite is not. See [PLAN.md](PLAN.md) for the phase plan.

## Why

The upstream suite is the right set of tests and the wrong deployment story: running it
means pinning a Python toolchain, resolving `boto3`/`botocore` versions that drift under
you, and shipping all of that into every CI job that wants a conformance gate. `s3t`
keeps the tests and drops the runtime — one binary, one config file.

It also runs the suite concurrently. Upstream's fixtures nuke every test bucket before
and after every test, which forces a serial run; `s3t` scopes each test to its own bucket
prefix so a worker pool is safe.

## Install

```console
go install github.com/go-faster/s3t/cmd/s3t@latest
```

Or grab a binary from [releases][releases], or use the container image:

```console
docker run --rm -v $PWD/s3tests.conf:/etc/s3t.conf \
  ghcr.io/go-faster/s3t run -c /etc/s3t.conf
```

## Usage

Copy `s3tests.conf.SAMPLE`, point it at your server, and run:

```console
s3t run -c your.conf
```

The config format is upstream's, unchanged — an existing `s3tests.conf` works as-is.

Select tests by name, by marker, or from a file of pytest node IDs:

```console
s3t run -k '^bucket_list'                   # like pytest -k
s3t run -m 'lifecycle and not fails_on_aws' # like pytest -m
s3t run --allow-list allow.txt              # gate on a fixed set
```

`list` and `markers` show what a selection covers without contacting a server, so
neither needs a config file:

```console
s3t list -m versioning
s3t list --node-ids        # the form allow-list files use
s3t markers
```

## Relationship to ceph/s3-tests

This is a port, not a fork or a replacement. Upstream remains the reference: it defines
what the tests mean, and it covers areas this port does not (IAM, STS, s3select, and the
RGW-specific extensions). When the two disagree about a result, upstream is right and
this is a bug — see `DIVERGENCE.md` for the known, deliberate exceptions.

The commit being tracked is recorded in [UPSTREAM](UPSTREAM).

## License

[MIT](LICENSE), the same license as the original suite, whose copyright notice is
retained. See [NOTICE](NOTICE) for attribution.

[upstream]: https://github.com/ceph/s3-tests
[releases]: https://github.com/go-faster/s3t/releases
