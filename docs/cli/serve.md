---
title: csi-bottle serve
description: Start the CSI driver server
---

<!--
This documentation is auto generated by a script.
Please do not edit this file directly.
-->

<!-- markdownlint-disable-next-line single-title -->
# csi-bottle serve

Start the CSI driver server

## Synopsis

Listens on a UNIX socket or TCP socket for GRPC method calls from Kubelet.

## Usage

```plaintext
csi-bottle serve [flags]
```

## Options

```plaintext
Options:
      --endpoint string        CSI endpoint to listen on (default "unix:///tmp/csi/csi.sock")
  -h, --help                   help for serve
      --name string            name of the driver (default "bottle.csi.act3-ace.io")
      --nodeid string          node id (default "nodeid")
      --pruneperiod duration   Time between pruning runs.  Examples: 1m, 1h, 12h, 72h (default 24h0m0s)
      --prunesize string       Max size of cache in bytes.  SI suffixes are allowed.  Examples: 1 Gi, 50 G, 1 Ti (default "10Gi")
      --storagedir string      Root path for the node local data storage (default "/tmp/csi/data")
      --telemetry string       URL of the Telemetry Server
```

## Options inherited from parent commands

```plaintext
Global options:
  -v, --verbosity strings[=warn]   Logging verbosity level (also setable with environment variable ACE_DATA_CSI_VERBOSITY)
                                   Aliases: error=0, warn=4, info=8, debug=12 (default [warn])
```
