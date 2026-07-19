# Transactional Builder Output

## Goal

Builder stages must not modify the last successful JSON dataset. They decode,
join, validate, and register output artifacts first. Only after every stage
succeeds are the artifacts encoded and published.

The public output layout remains flat. Existing consumers continue opening
`items.json`, `world.json`, and the other files directly from the configured
output directory.

## API

`src/output.Transaction` owns registration, staging, publication, and recovery.
Construction recovers an interrupted transaction before the caller starts
producing the next dataset:

```go
outputs, err := output.New(dir, output.NewGoccyJSONDriver(pretty))
if err != nil {
	return err
}
if err := outputs.Register("world.json", world); err != nil {
	return err
}
if err := outputs.RegisterExclusive("items.json", output.NewJSONArray(items)); err != nil {
	return err
}
return outputs.Publish()
```

The transaction is serialization-agnostic. `output.Driver` receives each staging
path and registered value, so another consumer can supply its own encoding or
file-production strategy. `GoccyJSONDriver` is the extractor's default;
`StandardJSONDriver` and `JettisonJSONDriver` provide equivalent behavior
through `encoding/json` and `wI2L/jettison`.
Exclusive values run alone and are intended for drivers that already parallelize
or have a large temporary allocation peak.

## Ownership

Each successful build records the relative paths it owns in
`.build-outputs.json`. Publication compares the previous and next ownership
manifests:

- paths in the next manifest are added or replaced;
- paths only in the previous manifest are removed;
- every other file and directory in the output directory is untouched.

When no previous ownership manifest exists, publication replaces the files in
the new manifest but does not remove unidentified files.

Artifact names must be clean relative paths. Absolute paths and paths that can
escape the output directory are rejected.

## Staging

Artifacts are written beneath a unique staging directory in the output
directory. A failed build stage creates no staging directory. A failed artifact
write removes its staging directory and leaves the active dataset unchanged.

Artifact writes use bounded concurrency. Large arrays may parallelize their own
JSON encoding, so bulk arrays run exclusively while smaller sidecars use bounded
outer concurrency.

## Publication

Publication uses a unique rollback directory and `.build-transaction.json` as a
journal. The journal contains the union of the previous and next owned paths and
whether each path existed before publication.

For each owned path, publication moves the previous file to rollback and moves
the staged replacement into its normal flat path. Paths removed by the new
manifest have no staged replacement. The ownership manifest is published as
part of the same transaction.

After every move succeeds, a separate atomic commit marker is published. Cleanup
then removes the rollback directory, staging directory, journal, and marker.
Cleanup may be retried.

## Recovery

Builder startup checks the journal before decoding game data:

- an uncommitted transaction is rolled back to the previous dataset;
- a committed transaction is completed as the new dataset;
- recovery operations are idempotent and may themselves be interrupted.

Unrelated user files are never moved or deleted. The pipeline provenance
`manifest.json`, icon output, and world-map output are outside the builder-owned
transaction unless explicitly registered as builder artifacts.

## Required Tests

- a stage failure never modifies existing output;
- an artifact-write failure never modifies existing output;
- unrelated files survive publication and recovery;
- files removed from the next ownership manifest disappear after commit;
- recovery restores every interruption point of an uncommitted publication;
- recovery completes every interruption point after commit;
- duplicate or unsafe artifact paths are rejected;
- artifact concurrency is bounded.
