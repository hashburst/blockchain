# HVM Network source recovery

This standalone Go module preserves the supplied source archive without overlaying the legacy repository layout. Run Go and devnet commands from this directory.

Repository parent: cdcf740fdc0a16eb695c9176142cf848c5e7b9ad (master).
Source: hashburst-hvm-network-source-review-20260923T001921Z.tar.gz.
Archive SHA256: e1edf570689ee744567f592328623e538501bd1ed68ef946cf827d28b74a7879.
SOURCE_ARCHIVE_SHA256SUMS records the 76 original source files before corrective commits. This archive is the supplied review subset, not a claim to recover absent contracts, documentation or operational data.

No database, node keys, operational journal, production configuration, binaries or generated devnet state is imported. Existing legacy files and Git history are preserved.

This import is for review and devnet validation. It does not deploy or activate HVM, regenerate public genesis, or change chain IDs.

The original archive and RC3 checksum manifests are preserved unchanged in Git history before the HVM Network rename. They are not checksums of the renamed source tree. Consult commit f7811f75ca1c1a5269d4f4faed0ee0cdcb2bd516 for the original filenames and evidence.
