# Divergences from ceph/s3-tests

Tests whose result here differs from the Python suite's against the same
server, with the reason. The bar from [PLAN.md](PLAN.md) §9 is zero
*unexplained* mismatches: a Go test that passes where Python fails is a bug in
the port until it has an entry on this page.

Each entry names the tests, which way the result differs, and why.

## SigV4 date headers: `Date` versus `X-Amz-Date`

- `test_headers.py::test_object_create_date_and_amz_date`
- `test_headers.py::test_object_create_amz_date_and_no_date`

**Pass here, fail under pytest.** Both are marked `fails_on_rgw` upstream.

The tests set a `Date` header, an `X-Amz-Date` header, or both, and then expect
an ordinary write to succeed. What actually reaches the server is decided by
the SDK, and the two SDKs decide differently.

botocore's SigV4 signer treats a caller-supplied `Date` header as the request
timestamp: it reformats `Date` and *deletes* `X-Amz-Date`
(`SigV4Auth._set_necessary_date_headers`). Since the hook sets `Date` in both
tests — to the empty string in the second, which still counts as present — the
request goes out carrying only `Date`. A server that requires `X-Amz-Date`
rejects it, which is what `fails_on_rgw` records.

`aws-sdk-go-v2` has no such mode. Its signer always writes `X-Amz-Date` and
signs it, leaving any `Date` header alone, so the request carries both and is
accepted.

Matching botocore would mean signing over `Date` instead of `X-Amz-Date`, which
the Go signer cannot be asked to do; deleting `X-Amz-Date` after signing would
only invalidate the signature and test nothing. The divergence is in the
signers, not in the port.
