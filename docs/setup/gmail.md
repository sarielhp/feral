# Gmail Account Setup

`feral` communicates directly with Google's REST APIs using OAuth2, without requiring local IMAP sync daemons (`mbsync` or `offlineimap`).

---

## The Open-Source OAuth Model

Because `feral` is an open-source terminal application, embedding a shared client secret would expose all users to Google API quota exhaustion and unverified app security barriers.

Instead, you register your own free, private **Google Cloud Desktop Application**. This ensures your access is completely private, unthrottled, and owned exclusively by you.

Setup takes approximately **3 minutes**.

---

## Step 1: Create a Google Cloud Project

1. Go to the [Google Cloud Console](https://console.cloud.google.com/).
2. Click the project dropdown at the top and select **New Project**.
3. Name your project (e.g., `mail-cli-personal`) and click **Create**.

---

## Step 2: Enable Required APIs

1. In the sidebar, navigate to **APIs & Services** -> **Enabled APIs & Services**.
2. Click **+ ENABLE APIS AND SERVICES**.
3. Search for and enable the following APIs:
   - **Gmail API**
   - **Google People API** (enables contact suggestions and address autocompletion)
   - **Google Calendar API** (optional, enables calendar invitation syncing)

---

## Step 3: Configure the OAuth Consent Screen

1. In the sidebar, navigate to **APIs & Services** -> **OAuth consent screen**.
2. Select User Type: **External** and click **Create**.
3. Fill in the required fields:
   - **App name**: `feral`
   - **User support email**: your Gmail address
   - **Developer contact information**: your Gmail address
4. Click **Save and Continue** through Scopes (no manual scopes needed here).
5. Under **Test users**, click **+ ADD USERS**, add your own Gmail address, and click **Save and Continue**.
6. **Publish the app** (**OAuth consent screen** -> **Publishing status** -> **Publish app**). While it
   stays in *Testing*, Google expires the refresh token for Gmail access after **7 days**, and you would
   have to sign in again every week. You do not need Google's verification for a client only you use:
   at sign-in Google shows a "Google hasn't verified this app" warning; choose **Advanced** ->
   **Go to feral (unsafe)** to continue. The app is your own, so the warning is expected.

---

## Step 4: Create Desktop OAuth Credentials

1. In the sidebar, navigate to **APIs & Services** -> **Credentials**.
2. Click **+ CREATE CREDENTIALS** at the top -> select **OAuth client ID**.
3. Under **Application type**, choose **Desktop App**.
4. Set Name to `feral desktop` and click **Create**.
5. Click **DOWNLOAD JSON** on the confirmation dialog.
6. Save the downloaded file to:
   ```bash
   ~/.config/feral/credentials.json
   ```

---

## Step 5: Configure and Authenticate in `feral`

Run the setup wizard:

```bash
feral account setup
```

1. Select option **`2`** (Gmail).
2. Enter your account name (e.g., `personal-gmail`).
3. Enter your Gmail address.
4. When prompted, confirm login. A browser window will open asking you to sign into your Google account and grant access to `feral`.
5. Once approved, `feral` saves the token in `~/.config/feral/token_<name>.json`. It is a plain
   file readable only by you (mode 600); treat it like a password, since it grants access to the mailbox.

Alternatively, add and authenticate directly:

```bash
feral account add gmail personal-gmail
feral account login personal-gmail
```

---

## Step 6: Verify Connection

```bash
feral account test personal-gmail
feral label list
feral %
```
