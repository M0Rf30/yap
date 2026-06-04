# SchemaStore submission for `yap.json`

This directory stages the files needed to register the `yap.json` schema in
[SchemaStore](https://github.com/SchemaStore/schemastore) so editors
(VS Code, JetBrains, Neovim, etc.) auto-validate any `yap.json` by filename —
no `$schema` line required by end users.

This folder is **not** part of the yap build; it's a copy-ready payload for a
SchemaStore fork.

## How to submit

1. Fork and clone `https://github.com/SchemaStore/schemastore`.
2. Copy files into the fork (paths mirror this tree):

   ```sh
   cp src/schemas/json/yap.json        <schemastore>/src/schemas/json/yap.json
   mkdir -p <schemastore>/src/test/yap <schemastore>/src/negative_test/yap
   cp src/test/yap/*.json              <schemastore>/src/test/yap/
   cp src/negative_test/yap/*.json     <schemastore>/src/negative_test/yap/
   ```

3. Add the catalog entry from `catalog-entry.json` into
   `<schemastore>/src/api/json/catalog.json` inside the `schemas` array,
   keeping the array **alphabetically sorted by `name`** (between entries
   starting with "y").

4. From the SchemaStore repo root, install and validate:

   ```sh
   npm install
   node ./cli.js check --schema-name=yap.json
   npm run prettier:fix
   ```

5. Commit and open a PR against `SchemaStore/schemastore`.

## Notes / compliance

- Schema uses `draft-07` (SchemaStore's recommended version).
- `$id` is `https://www.schemastore.org/yap.json` (repo-hosted convention).
- `fileMatch` is `["yap.json"]`.
- Positive samples: `src/test/yap/minimal.json`, `src/test/yap/full.json`.
- Negative sample: `src/negative_test/yap/missing-required.json`
  (omits required `buildDir`/`output`/`projects` and uses an invalid
  `compressionDeb` enum value).
- The canonical schema source of truth lives at the repo root
  (`yap.schema.json`); keep this copy in sync on any schema change.
