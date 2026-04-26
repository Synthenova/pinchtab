# Cloak + Cloud Profile Get Started

This guide is for the exact setup used in this repo:

- PinchTab server on `127.0.0.1:9868`
- CloakBrowser Chromium as the browser binary
- shared cloud-backed PinchTab profiles in GCS

Use this guide when an agent needs to set up a fresh machine end to end without human hand-holding:

1. download CloakBrowser
2. update PinchTab browser config
3. check for the GCS credential JSON
4. start the PinchTab server
5. import the cloud profiles
6. return the dashboard URL

This is intentionally operational and exact.

## Expected Result

At the end of this guide, the machine should have:

- CloakBrowser downloaded and cached locally
- PinchTab configured with the Cloak binary path and Chrome version
- PinchTab server running at `http://127.0.0.1:9868`
- cloud profiles imported into PinchTab
- dashboard URL ready to hand back to the user

## Inputs This Setup Uses

These are the shared cloud profile settings currently in use:

- bucket: `conthunt-dev-pinchtab-profiles`
- prefix: `pinchtab/profiles`

Current shared cloud profiles:

- `Comphy Headed`
  - cloud profile ID: `cp_aaff9877399d495686b5333ce426e63d`
- `Layer Headed`
  - cloud profile ID: `cp_fe7b48fc31fa45f08e063b4fc103835f`

Expected local GCS key path for automation on this repo:

- `/Users/nirmal/Desktop/pinchtab/gcs-bucket-ops.json`

If that file is not present on the target machine, do not guess. Ask the user for the cloud storage JSON key and stop until they provide it.

## 1. Download CloakBrowser

This repo already has the CloakBrowser source checked out at:

- `../CloakBrowser`

Use the JavaScript wrapper CLI because it has an explicit install command and info command.

From the PinchTab repo root:

```bash
cd ../CloakBrowser/js
npm install
npx cloakbrowser install
npx cloakbrowser info
```

What this does:

- downloads the CloakBrowser Chromium build into `~/.cloakbrowser/`
- prints the installed binary path and version information

From CloakBrowser’s own docs, the useful commands are:

```bash
npx cloakbrowser install
npx cloakbrowser info
npx cloakbrowser update
```

## 2. Set PinchTab Browser Binary and Chrome Version

PinchTab must be pointed at the Cloak binary through its main config.

The relevant config fields are:

- `browser.binary`
- `browser.version`

Current working example on this machine:

```json
{
  "browser": {
    "binary": "/Users/nirmal/.cloakbrowser/chromium-145.0.7632.109.2/Chromium.app/Contents/MacOS/Chromium",
    "version": "145.0.7632.109"
  }
}
```

Important:

- `browser.binary` is the full executable path
- `browser.version` is the four-part Chrome version
- if the Cloak cache dir is named `chromium-145.0.7632.109.2`, PinchTab version must be `145.0.7632.109`
- do not include the trailing package suffix like `.2` in `browser.version`

Update PinchTab config with either:

```bash
pinchtab config patch '{"browser":{"binary":"/ABSOLUTE/PATH/TO/CLOAK/BINARY","version":"145.0.7632.109"}}'
```

or edit the config file directly if needed:

- legacy path used on this machine: `~/.pinchtab/config.json`

To inspect the active config:

```bash
pinchtab config show
```

## 3. Check for the GCS Key

Before importing cloud profiles, verify the credential file exists.

Expected path for this setup:

```bash
test -f /Users/nirmal/Desktop/pinchtab/gcs-bucket-ops.json
```

If the file exists, continue.

If the file does not exist:

- stop immediately
- ask the user for the cloud storage JSON key
- do not proceed with cloud profile import until the file is available

For this setup, the import credential path should be:

- `/Users/nirmal/Desktop/pinchtab/gcs-bucket-ops.json`

If the user provides the key at a different path, use that exact path consistently in the import requests.

## 4. Start the PinchTab Server

From the PinchTab repo root:

```bash
./pinchtab server
```

Expected URL:

- `http://127.0.0.1:9868`

If you need the API token for scripted import:

```bash
jq -r '.server.token' ~/.pinchtab/config.json
```

## 5. Import the Cloud Profiles

The clean automation path is to use the cloud import API directly.

### 5.1 Optional: discover available cloud profiles first

```bash
TOKEN=$(jq -r '.server.token' ~/.pinchtab/config.json)

curl -s -X POST http://127.0.0.1:9868/profiles/cloud/discover \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "bucket": "conthunt-dev-pinchtab-profiles",
    "prefix": "pinchtab/profiles",
    "credentialPath": "/Users/nirmal/Desktop/pinchtab/gcs-bucket-ops.json"
  }'
```

### 5.2 Import `Comphy Headed`

```bash
TOKEN=$(jq -r '.server.token' ~/.pinchtab/config.json)

curl -s -X POST http://127.0.0.1:9868/profiles/cloud/import \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Comphy Headed",
    "bucket": "conthunt-dev-pinchtab-profiles",
    "prefix": "pinchtab/profiles",
    "credentialPath": "/Users/nirmal/Desktop/pinchtab/gcs-bucket-ops.json",
    "profileId": "cp_aaff9877399d495686b5333ce426e63d",
    "keepLocalCache": true
  }'
```

### 5.3 Import `Layer Headed`

```bash
TOKEN=$(jq -r '.server.token' ~/.pinchtab/config.json)

curl -s -X POST http://127.0.0.1:9868/profiles/cloud/import \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Layer Headed",
    "bucket": "conthunt-dev-pinchtab-profiles",
    "prefix": "pinchtab/profiles",
    "credentialPath": "/Users/nirmal/Desktop/pinchtab/gcs-bucket-ops.json",
    "profileId": "cp_fe7b48fc31fa45f08e063b4fc103835f",
    "keepLocalCache": true
  }'
```

## 6. What the Agent Should Return

After setup is complete, the agent should return:

- server URL: `http://127.0.0.1:9868`
- confirmation that CloakBrowser binary path was configured
- confirmation that the GCS key was present and used
- confirmation that `Comphy Headed` and `Layer Headed` were imported

Example final handoff:

```text
PinchTab is running at http://127.0.0.1:9868
CloakBrowser binary is configured in PinchTab
GCS credentials were found and cloud profiles were imported:
- Comphy Headed
- Layer Headed
```

## Notes

- Deleting a cloud-backed PinchTab profile removes only the local entry. It does not delete the cloud copy in GCS.
- That means the profile can always be attached again later with `Import Cloud`.
- The cloud storage format uses:
  - `meta.json`
  - `latest.json`
  - `versions/<version>.tar.gz`
- For first-time sync, a profile must be started and then stopped cleanly before its first snapshot exists in GCS.
