# NetBird daemon gRPC contract

This package vendors the NetBird daemon gRPC contract used by the NetworkManager plugin.

Source:
- module: `github.com/netbirdio/netbird`
- version: `v0.70.4`
- commit: `3fc5a8d4a1fe308ff1068764a09b90b0859ab8fe`
- upstream files: `client/proto/daemon.proto`, `client/proto/daemon.pb.go`, `client/proto/daemon_grpc.pb.go`

Only the daemon API contract is copied here; the full NetBird repo is not vendored or added as a submodule.

The copied files are from NetBird's BSD-3-Clause-covered `client/proto` tree. See `LICENSE.netbird`.

The generated Go package name was changed from upstream `proto` to local `daemonproto` so imports are unambiguous inside this module.
