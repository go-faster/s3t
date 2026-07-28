# s3t — Go rewrite of ceph/s3-tests

Port of `/src/s3-tests` (Python/pytest/boto3) to Go, as a **standalone CLI harness**
with **behavior-identical, idiomatic** tests.

## Scope

Source suite: ~30k lines, ~980 tests across 6 modules.

| Module | Lines | Tests | In scope |
|---|---:|---:|---|
| `functional/test_s3.py` | 20827 | 760 | **yes** |
| `functional/test_headers.py` | 578 | 48 | **yes** |
| `functional/test_iam.py` | 3028 | 90 | deferred |
| `functional/test_sts.py` | 2168 | 37 | deferred (needs Keycloak) |
| `functional/test_s3select.py` | 1705 | 38 | deferred |
| `functional/test_sns.py` | 186 | 5 | deferred |
| `functional/test_s3control.py` | 90 | 1 | deferred |

**Correction to the encryption cut.** 30 of the 245 gate tests are SSE-C and
SSE-KMS tests, and they pass on `go-faster/fs` — not because it implements
encryption, but because it ignores the SSE headers and round-trips the data. The
cut below was justified as "`fs` does not implement it", which does not hold for
that subset, so those 30 are in scope as part of the gate. The rest of the
encryption area stays cut.

Within those two files, two areas are **cut** (decided, see §8):

| Area | Tests | Why cut |
|---|---:|---|
| encryption — SSE-C, SSE-KMS, SSE-S3, bucket encryption | ~100 | `go-faster/fs` does not implement it; would sit red against the only backend in CI |
| bucket logging | ~118 | RGW extension; needs a bespoke XML layer for types the Go SDK lacks |

The in-scope target is therefore **~590 tests**. The harness, config, and client factory
are built to accommodate both the cut areas and the deferred modules (IAM/STS/SNS clients
are part of the factory from day one), so picking either up later is test-writing only,
no rework.

Upstream HEAD being ported: `5522d1c`. Record this in `UPSTREAM` so future rebases can
diff `git log 5522d1c..` for new/changed tests. This is the same commit `go-faster/fs`
already pins as `S3TESTS_REF`.

## Consumer

**`go-faster/fs`** — an S3-compatible server in the same org, which today runs the Python
suite in two CI workflows (`.github/workflows/s3tests.yml`, `s3tests-cluster.yml`) against
a freshly built binary. Its current standing (measured, from `findings/S3TESTS.md`):

| Suite file | Collected | Pass | Fail | Skip |
|---|---:|---:|---:|---:|
| `test_s3.py` | 838 | 253 | 491 | 94 |
| `test_headers.py` | 48 | 22 | 26 | 0 |

CI gates on a hand-curated allow-list of **245 node IDs** (`.github/s3tests/allow.txt`,
all green, verified deterministic over two clean-server runs); the full suite runs weekly
for information only, so newly-passing tests can be promoted into the list.

This makes the real first milestone **the 245 allow-listed tests, not all 808** — that is
the set that has to be green for `s3t` to replace pytest in the gating workflow. It also
fixes three requirements that would otherwise have been guesses:

- **Test names must match upstream exactly.** `allow.txt` holds pytest node IDs
  (`s3tests/functional/test_s3.py::test_abort_multipart_upload`). `s3t` accepts that file
  verbatim — `--allow-list <file>`, parsing `#` comments and blank lines the same way,
  mapping `<path>::<name>` onto registered names. Drop-in: the workflow's `mapfile`/`sed`
  dance is replaced by one flag, and the allow-list file itself needs no edits.
- **JUnit XML output is mandatory**, not a nice-to-have — the weekly job uploads it as the
  promotion mechanism.
- **Speed target.** The whole Python `test_s3.py` run takes 67s against `fs`. `s3t` must
  beat that comfortably or the port is a regression in developer experience; with the
  worker pool it should land in single-digit seconds for the allow-list.

Everything not in the allow-list is still worth porting — the 491 failures are the map for
`fs`'s roadmap — but they are not the gate.

## Non-goals

- No `go test` integration. The deliverable is a `s3t` binary (decision: standalone harness).
- No boto2 legacy paths — upstream already dropped them.
- Not re-deriving the S3 spec. The Python suite is the spec; divergences get documented.

---

## 0. Public repository

Ships as **`github.com/go-faster/s3t`**, public.

### Licensing — decided: MIT everywhere

Upstream `ceph/s3-tests` is **MIT** (New Dream Network, LLC, 2011). A translation of its
test logic is a derivative work, so the MIT notice must be retained — an obligation, not
a courtesy. The whole repo is therefore MIT, diverging from the `go-faster` house
Apache-2.0 default in exchange for a single license with nothing for a contributor or
downstream user to reason about.

`LICENSE` carries both copyright lines (New Dream Network 2011, go-faster 2026); `NOTICE`
credits `ceph/s3-tests` with the pinned commit; the README says plainly that this is a
port and where the original lives.

### Repo hygiene

- Reuse the org's shared workflows (`go-faster/x/.github/workflows/{x,commit}.yml`) as
  `template` does, plus `dep-review` and `codecov` — consistent with the other 20 repos.
- `.goreleaser.yaml` (6 siblings already have one): `s3t` is a CLI other people will
  actually run, so tagged releases publish binaries for linux/darwin × amd64/arm64 and a
  `ghcr.io/go-faster/s3t` image. Running a conformance suite should not require a Go
  toolchain.
- README carries the badge row, a 5-line quickstart, and the **compatibility matrix** —
  see below.
- `s3tests.conf.SAMPLE` contains upstream's fake credentials. They are published upstream
  already, but secret scanners will flag them; add a `.gitleaks.toml` allowlist with a
  comment explaining why, rather than leaving contributors to rediscover it.

### CI has to bring its own server

The Python suite could assume a Ceph vstart cluster. A public repo cannot — there is no
backend on a GitHub runner. So `s3t`'s own CI builds `go-faster/fs` from source (same org,
single static binary, ~seconds) and runs the suite against it, gating on the same
245-test allow-list. That gives every PR a real end-to-end run without a service container
or credentials. A second informational job runs the full suite so `s3t` regressions and
`fs` behavior changes are distinguishable.

### The compatibility matrix is the reason to be public

437 of the markers are `fails_on_aws` / `fails_on_dbstore` / `fails_on_rgw` — upstream's
record of where implementations legitimately diverge. A public runner that emits per-
backend results makes that legible: publish a generated table of which backends pass what.
That is the thing that makes `s3t` useful to people outside the org, and it costs one
report formatter on top of work already planned.

---

## 1. Repository layout

```
/src/faster/s3t/
  go.mod                       module github.com/go-faster/s3t   (go 1.26)
  LICENSE  NOTICE              see §0 — MIT retained from ceph/s3-tests
  README.md                    badges, quickstart, compatibility matrix
  Makefile  .golangci.yml  .goreleaser.yaml  .github/workflows/
  UPSTREAM                     pinned s3-tests commit this port tracks
  s3tests.conf.SAMPLE          copied verbatim from upstream

  cmd/s3t/main.go              thin main, calls internal/cmd.Root()
  internal/cmd/                cobra commands
    root.go  run.go  list.go  markers.go
  internal/suite/suite.go      concatenates every tests/* package's Tests() (single seam)

  internal/harness/            registry, T, scheduler, reporting
    registry.go  t.go  run.go  markers.go  report.go  report_junit.go
  internal/config/             INI parse of S3TEST_CONF -> Config
  internal/client/             SDK client factory, middleware, sigv2 signer
    factory.go  headers.go  sigv2.go  errors.go
  internal/fixture/            bucket naming, new-bucket, nuke, per-test cleanup
  internal/rawhttp/            unsigned/raw requests, presign, POST-object forms
  internal/s3util/             assertions, random data, policy builders, body read

  tests/s3/                    port of test_s3.py, split by area (~15 files)
  tests/headers/               port of test_headers.py
```

## 2. CLI

`spf13/cobra` v1.10, following the layout used across the other `go-faster` repos:
`cmd/s3t/main.go` is a thin `os.Exit(cmd.Root().Execute())`, and `internal/cmd/root.go`
exposes `func Root() *cobra.Command` composing subcommands.

```
s3t run [flags]      run the selected tests (default command)
s3t list [flags]     print selected test names, one per line, and exit
s3t markers          print every registered marker with its test count
s3t version          build info from debug.ReadBuildInfo
```

Persistent flags on the root:

| Flag | Default | Meaning |
|---|---|---|
| `--config`, `-c` | `$S3TEST_CONF` | path to the INI config; flag wins over env |
| `--run`, `-k` | `` | regexp over test names (pytest `-k`) |
| `--markers`, `-m` | `` | marker expression, e.g. `'lifecycle and not fails_on_aws'` |
| `--allow-list` | `` | file of pytest node IDs to run; `#` comments and blanks ignored |

`--allow-list` exists so `go-faster/fs` can point at its existing
`.github/s3tests/allow.txt` unedited (see Consumer above). Node IDs are
`<path>::<test_name>`; the path is validated against the module the test came from and a
mismatch is an error, not a silent skip — a typo in the gate file must fail loudly. An
entry naming a test that does not exist is likewise an error, so the list cannot rot into
passing vacuously. `--allow-list` intersects with `--run`/`--markers` rather than
overriding them.

`run`-only flags: `--parallel/-p` (default `min(8, NumCPU)`), `--serial`,
`--timeout` (per test, default `5m`), `--json <file>`, `--junit <file>`, `--fail-fast`,
`--verbose/-v` (stream per-test logs instead of buffering until failure).

Sharing `--run`/`--markers` across `run`, `list`, and `markers` is the point of putting
them on the root: `s3t list -m lifecycle` previews exactly what `s3t run -m lifecycle`
will execute. Selection flags are bound to one `selectOpts` struct that both subcommands
resolve, so there is a single code path from flags to selected tests.

Config loading and client construction happen in `RunE`, not `init()` — so `s3t list`
and `s3t markers` work with no config file and no reachable endpoint. `RunE` returns
errors rather than exiting, with `SilenceUsage`/`SilenceErrors` set so a mid-run failure
does not dump usage text. `run` exits 1 when any test fails.

Cobra's `context.Context` (`cmd.Context()`) is the root of every test's `t.Ctx()`, wired
to `signal.NotifyContext` so Ctrl-C cancels in-flight requests and still runs cleanups
and prints the summary.

## 3. Harness design

Replaces pytest. **No global registry and no `init()` registration**: each test package
returns its tests as a plain slice (`func Tests() []harness.Test`), `internal/suite`
concatenates them, and `internal/cmd` collects the result into a `harness.Registry`. One
seam to touch when a package is added, no import-order dependency to reason about, and a
harness test can build a registry in isolation.

```go
package harness

type Test struct {
    Name    string          // "bucket_list_empty" — matches the Python name exactly
    Module  string          // upstream file, so allow-list node IDs can be validated
    Markers []string        // "fails_on_aws", "lifecycle", ...
    Fn      func(*T)
}

func (t Test) NodeID() string              // "…/test_s3.py::test_bucket_list_empty"

type Registry struct{ /* ... */ }
func NewRegistry(tests []Test) (*Registry, error)  // rejects dupes, empty fields, bad module

type T struct{ /* ... */ }
func (t *T) Ctx() context.Context          // cancelled on timeout/interrupt
func (t *T) Errorf(format string, a ...any) // mark failed, continue
func (t *T) Fatalf(format string, a ...any) // mark failed, unwind via panic(failNow{})
func (t *T) Skipf(format string, a ...any)
func (t *T) Cleanup(fn func())             // LIFO, runs even on failure
func (t *T) Logf(format string, a ...any)  // buffered, printed only on failure
```

- **Failure unwinding**: `Fatalf` panics with a sentinel recovered by the worker;
  any other panic is a test failure with stack. Mirrors `t.Fatal` semantics so ported
  assertions read naturally.
- **Assertions** live in `s3util` and take `*T`: `s3util.EqualStatus(t, err, 404,
  "NoSuchKey")`, `s3util.Equal`, `s3util.NoError`. Keeps `testify` out (it wants
  `testing.TB`), no reflection surprises.

### Selection

Driven by the root flags above. `--run` is a regexp over test names; `--markers` goes
through a real expression parser supporting `not X`, `X and Y`, `X or Y` and parentheses,
reproducing `-m 'bucket_logging and not fails_without_logging_rollover'`. Both are pure
functions over the registry — no config or network needed, which is what lets `list`
work standalone.

### Execution

- Worker pool sized by `--parallel`, `--timeout` per test.
- **Critical divergence from Python, deliberate**: upstream's `setup_teardown` autouse
  fixture nukes *every* bucket matching the global prefix before and after *every*
  test — O(n²) and strictly serial. Here each test gets its **own prefix**
  `<cfgprefix><testslug>-` and cleanup nukes only that prefix, via `t.Cleanup`. This is
  what makes `-p N` sound. Tests that assert over `list_buckets` output (`list_buckets*`,
  `bucket_list_distinct`) filter by their own prefix, as upstream's `get_buckets_list`
  already does.
- `--serial` forces one worker for debugging; a `serial` marker pins individual tests to
  the same treatment (the `atomic_*` races and the lifecycle-timing tests).

Full concurrency and hang-resistance design is section 4.

### Reporting

- TTY: live counter + failure detail as it happens.
- Final summary: pass/fail/skip/error counts, wall time, slowest 10.
- `--json`: one JSON object per test (name, markers, status, duration, output).
- `--junit`: JUnit XML for CI.
- Exit 1 on any failure.

## 4. Concurrency and timeouts (anti-hang)

A conformance suite points at servers that are, by definition, possibly broken. The
harness must never be the thing that hangs: **every wait is bounded, and the bound is
enforced from outside the code doing the waiting.** Python's suite has none of this — a
wedged RGW hangs pytest until CI kills the job, with no report at all.

### Layered deadlines

Four nested budgets, each strictly tighter than the one above:

| Layer | Flag | Default | Enforced by |
|---|---|---|---|
| per test | `--timeout` | `5m` | `context.WithTimeout` per test + watchdog (below) |
| per test cleanup | `--cleanup-timeout` | `1m` | bounded separately, so a hung teardown cannot stall the worker a timed-out test just freed |
| per HTTP request | — | `60s` | `http.Client.Timeout` on the SDK transport |
| dial / TLS / response header | — | `10s` / `10s` / `30s` | `net.Dialer` + `http.Transport` fields |

*Built.* A whole-run `--deadline` was dropped: `--stall-timeout` and the per-test bound
already guarantee termination, and a second global clock is a knob with no failure mode
of its own to catch. The HTTP bounds are not flags yet — nothing has needed to change
them, and `client.Timeouts` is a struct the moment something does.

Response-header timeout is the important one: it catches a server that accepts the
connection and then never answers, which `http.Client.Timeout` alone handles poorly for
streaming bodies. Body reads get an explicit deadline via a wrapping reader rather than
relying on the client timeout, so a 5 GiB multipart download is not killed for being
large while a stalled 1 KiB read still fails fast.

### The watchdog

`context` cancellation only works if the test respects it. A test blocked on a
non-context-aware call, an unbounded `io.Copy`, or a `sync.WaitGroup` that never
completes will ignore its deadline. So the scheduler does not trust workers:

- Each test runs on its own goroutine; the worker waits on `select { case <-done;
  case <-time.After(timeout + grace) }`.
- On expiry the test is **reported as failed with a `timeout` status and its full
  goroutine stack**, and the worker moves on. The stuck goroutine is abandoned, not
  waited for — Go cannot kill a goroutine, and blocking the worker on it would let one
  bad test stall the run.
- Abandoned goroutines are counted; a nonzero count is printed in the summary and, past
  `--max-leaked` (default 8), the run aborts rather than leaking unbounded resources.
- A test that times out gets its cleanups run on a **separate** goroutine with its own
  short budget, so bucket deletion still happens for the common case where the test is
  stuck on one request rather than truly wedged.

### Stall detection

A global watchdog ticks every 30s. If **no test has finished** in `--stall-timeout`
(default `3× --timeout`), the run is considered wedged: dump all goroutine stacks, write
whatever report exists, exit non-zero. This is the backstop for a bug in the scheduler
itself, and it is the difference between a CI job that fails in 15 minutes with a stack
trace and one that burns its 6-hour limit.

*Built — and it immediately earned its keep.* Writing its test found that the scheduler
blocked in `wg.Wait()` and on the work channel, so with every worker wedged the detector
fired, recorded its error, cancelled the context — and `Run` still never returned. The
harness hanging is precisely what this section exists to prevent. Both the feed and the
wait now select on the context and abandon their workers.

### Concurrency model

- Worker pool over a channel of tests, not a goroutine per test — bounds in-flight HTTP
  connections, which matters against a single RGW endpoint.
- `--parallel` defaults to `min(8, NumCPU)`. The suite is I/O-bound, so higher values
  help, but past ~16 workers RGW starts returning 503 `SlowDown` and tests fail for the
  wrong reason. Documented, not silently clamped.
- **Serial tests run first, in one worker, before the parallel pool starts.** The
  `atomic_*` tests race concurrent readers against writers on one key and assume no
  other load; lifecycle tests sleep on wall-clock intervals and are sensitive to a
  loaded server. Interleaving them with 8 workers makes them flaky, which is worse than
  making them slow.
- Transport is shared across all clients with a raised `MaxIdleConnsPerHost` (default 2
  would serialize the pool onto two connections and quietly cap throughput at any
  `--parallel`).
- Test isolation is by bucket prefix (section 3), so workers never contend on the same
  server-side object — the only shared state is the endpoint itself.

### Retry and backoff

SDK retries stay off (they would mask the 5xx assertions the suite makes). Instead the
*fixture* layer retries — only bucket creation and cleanup, only on `SlowDown`/`503`,
bounded exponential backoff with jitter, capped at 30s total. This keeps retry logic out
of the assertions while stopping a loaded server from producing spurious setup failures.

### Interrupt handling

First Ctrl-C cancels the root context: in-flight requests fail, cleanups run, the report
prints, exit 130. Second Ctrl-C exits immediately. Without the two-stage behavior an
impatient interrupt during a slow cleanup leaves hundreds of buckets behind.

## 5. Config

`S3TEST_CONF` points at the same INI file as upstream — the sample is copied verbatim so
existing configs work unchanged. Notes:

- Python's `RawConfigParser` `[DEFAULT]` section inherits into all sections; keys contain
  spaces (`bucket prefix`) and section names contain spaces (`s3 alt`, `iam root`).
  `gopkg.in/ini.v1` handles all three; a hand-rolled parser would need to.
- `Config` is a plain struct with typed fields (`MainAccessKey`, `LCDebugInterval`, …)
  and the same fallback defaults as `configure()` (`lc_debug_interval=10`,
  `kms_keyid=testkey-1`, …).
- `choose_bucket_prefix` — pads `{random}` to 30 chars — ported as-is.

## 6. S3 client factory

`aws-sdk-go-v2` (`service/s3`, plus `sts`/`iam`/`sns` for the deferred phase). Chosen
over `minio-go` because its **middleware stack is the direct analogue of boto3's
`client.meta.events.register`**, which the Python suite uses 77 times to inject/strip
headers — that pattern has no equivalent in minio-go.

Profiles mirroring `functional/__init__.py`: `Main`, `Alt`, `Tenant`, `IAM`, `IAMRoot`,
`IAMAltRoot`, `Cloud`, `Unauthenticated`, `BadAuth`, `V2` (sigv2), `V2Tenant`.

Settings that must be pinned to match boto3:

- `UsePathStyle: true` — boto3 with `endpoint_url` defaults to path addressing.
- `RequestChecksumCalculation: WhenRequired` and `ResponseChecksumValidation: WhenSupported`.
  **Without this the modern Go SDK adds a CRC32 header and `aws-chunked` framing to every
  PutObject**, which changes request bytes and breaks the header/auth/checksum tests. The
  11 `checksum`-marked tests opt back in explicitly.
- Region from `api_name` (often empty); retries disabled (`aws.NopRetryer`) — the Python
  client does not retry, and retries would mask 5xx assertions.

### Header injection / removal

```go
client.WithHeaders(map[string]string{"Content-Length": "bad"})  // per-call option
client.WithoutHeader("Content-Length")
```
Implemented as `func(*middleware.Stack) error` in the Finalize step, passed via
`s3.Options.APIOptions` on a per-request basis — direct replacement for
`meta.events.register('before-call.s3.PutObject', ...)`. This must run **after** signing
for the malformed-header tests, and **before** signing where the test expects the bad
header to be signed; both variants are provided (`WithHeadersPreSign`).

### SigV2

26 tests use `get_v2_client`. **`aws-sdk-go-v2` removed SigV2 entirely** — needs a
hand-written signer (HMAC-SHA1 over the canonical `StringToSign`, `x-amz-` header
canonicalization, sub-resource query allowlist) installed as a Finalize middleware
replacing the v4 signer. ~200 lines, self-contained, unit-testable against known vectors
from the Python suite. Same code backs presigned-v2 URL generation.

### Errors

`s3util.StatusAndCode(err) (int, string)` unwraps `smithy.APIError` +
`awshttp.ResponseError`, mirroring `_get_status_and_error_code`. Note the Go SDK
sometimes surfaces `"api error <Code>"` where boto3 gives a bare code — normalize in one
place, not per test.

## 7. Things with no direct Go equivalent

| Python mechanism | Uses | Go approach |
|---|---:|---|
| `meta.events.register` header hooks | 77 | middleware (above) |
| `boto3.resource('s3')` / `bucket.objects.all()` | 72 | plain `ListObjectsV2` loop in `fixture` |
| `requests.post/get` raw HTTP | 91 | `internal/rawhttp` on `net/http` |
| POST-object form + policy signing | 36 | `rawhttp.PostForm` builder: base64 policy, HMAC-SHA1 (v2) and HMAC-SHA256 (v4) signatures, `multipart.Writer` with **field order preserved** (upstream uses `OrderedDict`; RGW is order-sensitive on `file` being last) |
| `generate_presigned_url/post` | 10 | `s3.NewPresignClient` (v4); custom for v2 |
| `threading` (atomic read/write races) | 17 | goroutines + `errgroup`; these tests run serial-pinned |

### Bucket logging — cut, and why it would have been expensive

Recorded because it is the reason the area was cut, and it will resurface if anyone
proposes picking it up. The Python suite exercises Ceph-only fields `LoggingType`,
`RecordsBatchSize`, `ObjectRollTime` on `PutBucketLogging`/`GetBucketLogging`, which
upstream enables by dropping `service-2.sdk-extras.json` into the botocore model dir. The
Go SDK's generated types have no such fields and cannot be extended the same way, so those
118 tests need a hand-built `<BucketLoggingStatus>` XML layer over sigv4-signed raw
requests — infrastructure the other ~590 tests do not need, for a feature `fs` does not
implement.

---

## 8. Porting phases

Each phase: port → run against a live endpoint → diff against the Python result for the
same tests → fix → commit. Test names stay identical to Python, which is what makes the
diff mechanical.

Ordering is driven by the allow-list, not by the layout of `test_s3.py`. **Phase 2 is
"whatever the 245 gating node IDs need, in allow-list order"** — that reaches a
swap-in replacement for `fs`'s gating workflow in the shortest path, and the remaining
phases fill in behind it. Concretely: sort `allow.txt` by area, port area by area,
and the milestone is `s3t run --allow-list allow.txt` green against `fs`.

| # | Content | Tests | Notes |
|---|---|---:|---|
| 0 | Repo skeleton, go.mod, license/NOTICE, Makefile, lint, CI, goreleaser | — | §0; CI builds `go-faster/fs` as its backend |
| 1a | cobra CLI + registry + `T` + config + client factory + fixtures + 20 smoke tests | 20 | |
| 1b | scheduler: worker pool, layered deadlines, watchdog, stall detector, interrupt handling, colorized reporting | — | **done.** fault-injection tested (§9); everything after is mostly mechanical |
| **2** | **the 245 allow-listed tests** — buckets, objects, ranged/multipart reads, copy, `delete_objects`, list v1/v2, multipart semantics | **245** | **done.** `fs` runs `s3t` in `s3tests.yml`, gated on a deny-list |
| 3 | remainder of bucket/object/list coverage not in the gate | ~120 | |
| 4 | ACLs (`bucket_acl`, `object_acl`, `access_bucket`) + `test_headers.py` + **sigv2** | ~130 | ACLs done; `test_headers.py` `auth_common` done. sigv2 signer and the `auth_aws2` half remain |
| 5 | versioning, delete markers, object lock | ~60 | |
| 6 | bucket policy, CORS, block-public-access, POST-object, `object_raw`, presign | ~120 | `rawhttp` lands here |
| 7 | lifecycle (+expiration/transition), storage classes, tagging, conditional write, checksum | ~110 | slow: real lifecycle waits, `lc_debug_interval` |
| — | ~~encryption~~ | ~~100~~ | **cut** — `fs` does not implement it |
| — | ~~bucket logging~~ | ~~118~~ | **cut** — RGW extension, needs `internal/cephext` |
| 8 | *(deferred)* IAM, STS, SNS, s3select, s3control | ~170 | harness already supports |

Rough output: **~15–18k lines of Go** for phases 1–7. Phases 0–2 are the ones with a
deadline attached; everything after is incremental and independently shippable.

The two cut areas total 218 tests that would have been ported to sit red against the only
backend in CI. They stay valuable against Ceph and real AWS, so the harness keeps room for
them — but `internal/cephext` is not built, and nothing in phases 0–7 depends on it.

## 9. Verification

The correctness gate is a **differential run**, not review:

1. Point both suites at the same `go-faster/fs` instance, built from source, using
   `fs`'s own `.github/s3tests/s3tests.conf` and `server.yaml` so the two runs see an
   identical server.
2. `pytest --json-report` on the Python suite, `s3t run --json` on ours.
3. `hack/compare.go` joins on test name and reports: tests present in one and not the
   other, and status mismatches (pass↔fail).
4. Target: **zero unexplained status mismatches**. Every remaining one gets an entry in
   `DIVERGENCE.md` with the reason (e.g. SDK adds a header boto3 does not).

`fs`'s published baseline (253/491/94 on `test_s3.py`, 22/26 on `test_headers.py`) is the
expected result, which makes this stronger than a self-consistency check: a Go test that
passes where Python fails is a **bug in the port**, not a win, and gets treated as one.
The 245 allow-listed tests are additionally required to be deterministic over two runs
against a clean server — the same bar `fs` used to admit them in the first place.

Additionally: `internal/` packages get ordinary `go test` unit tests — the sigv2 signer,
POST-policy construction, marker-expression parser, and INI parsing are all pure
functions with known-good vectors extractable from the Python source.

The anti-hang machinery is verified by **fault injection**, since it is exactly the code
that never runs on a healthy endpoint. Fake tests that block forever, ignore their
context, panic, or hang in cleanup assert that the run still terminates, the report is
complete, and the exit code is right; an `httptest` server that accepts and never
responds covers the transport bounds.

*Built.* `internal/harness` and `internal/client` cover: wedged test abandoned, run
continues past it, abort past `MaxLeaked`, cleanups still run for an abandoned test,
hung cleanup bounded, stall detector fires, pool bounded, serial tests never overlap,
results keep input order, response-header timeout, no retries. Whole suite runs in
~2.5s under `-race` with no real network.

Two lessons worth keeping:

- **A fault-injection test can pass vacuously.** The transport-timeout tests initially
  used a config with no credentials, so the SDK failed before reaching the network and
  "it errored quickly" was satisfied without any timeout firing. They now assert a
  *lower* bound on elapsed time too.
- **Don't ship an environment-dependent test.** A dial-timeout test against TEST-NET-3
  failed because the local network resets those packets in 36ms rather than blackholing
  them. Asserted structurally instead: a flaky test in CI is worse than no test.

## 10. Open items

- ~~Which backend do we validate against?~~ **Resolved: `go-faster/fs`** (see Consumer).
  Ceph vstart stays the secondary reference — it is the only backend where the
  RGW-extension tests can pass at all — but it is not what CI runs.
- ~~License?~~ **Resolved: MIT everywhere** (§0).
- ~~Do we port encryption and bucket logging?~~ **Resolved: no** — 218 tests cut (§8).
- **`fails_on_dbstore` / `fails_on_aws` (437 markers)** carry over as markers only; they
  are backend-selection metadata, not skips.
- **Cloud transition/restore** (14 tests) needs a second S3 endpoint (`[s3 cloud]`);
  they self-skip when unconfigured, same as upstream.
