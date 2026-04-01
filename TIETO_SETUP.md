# Connect your AI assistant to Tieto's Public 360° wiki

This guide helps you connect Claude, Cursor, or VS Code to Tieto's wiki at **wiki.software-innovation.com**. Don't worry if you're new to this — we'll walk through everything step by step!

---

## What you'll be able to do

Once connected, you can ask your AI assistant things like:
- "What does the wiki say about eFormidling?"
- "Find all pages about AutoSaver"
- "Who edited the Release Notes last week?"
- "Give me a quick overview of the Configuration page"
- "Find pages not updated in the last 90 days"

---

## Before you start

You'll need:
1. **One of these tools** installed on your computer:
   - [Claude Desktop](https://claude.ai/download) (Mac or Windows)
   - [Claude Code](https://docs.anthropic.com/en/docs/claude-code/overview) (command line)
   - [Cursor](https://www.cursor.com/) (code editor)
   - [VS Code](https://code.visualstudio.com/) with the [Copilot Chat](https://marketplace.visualstudio.com/items?itemName=GitHub.copilot-chat) extension
2. **A Tietoevry account** to access the wiki
3. **10 minutes** of your time

---

## Step 1: Download the server

This is a small program that lets your AI assistant talk to the wiki.

### Mac users

1. Go to the [Releases page](https://github.com/olgasafonova/mediawiki-mcp-server/releases/latest)
2. Download the right file for your Mac:
   - **Apple Silicon** (M1/M2/M3/M4): `mediawiki-mcp-server-mac-apple-silicon`
   - **Intel Mac**: `mediawiki-mcp-server-mac-intel`
3. Save it to your **Downloads** folder
4. Open **Terminal** (press Cmd+Space, type "Terminal", press Enter)
5. Run these two commands:
   ```bash
   mv ~/Downloads/mediawiki-mcp-server-mac-* ~/Downloads/mediawiki-mcp-server
   chmod +x ~/Downloads/mediawiki-mcp-server
   ```

Not sure which Mac you have? Click the Apple menu, then "About This Mac". If it says M1, M2, M3, or M4 — pick Apple Silicon. Otherwise pick Intel.

### Windows users

1. Go to the [Releases page](https://github.com/olgasafonova/mediawiki-mcp-server/releases/latest)
2. Download: `mediawiki-mcp-server-windows.exe`
3. Save it to your **Downloads** folder
4. You're done with this step!

---

## Step 2: Set up your wiki access

The Tieto wiki requires authentication for all operations, including reading. You'll need to create a bot password.

1. Open your browser and go to: [Special:BotPasswords](https://wiki.software-innovation.com/wiki/Special:BotPasswords)
2. Log in with your Tietoevry account
3. Type a bot name: **wiki-MCP**
4. Check these two boxes:
   - ✅ **Basic rights**
   - ✅ **Edit existing pages**
5. Click **Create**
6. **IMPORTANT:** Copy the generated password and save it somewhere safe. You won't see it again.

Your credentials will be:
- **Username:** `your.name@tietoevry.com#wiki-MCP`
- **Password:** the bot password you just saved

---

## Step 3: Connect to the wiki

Pick the tool you're using:

| I use... | Jump to |
|----------|---------|
| Claude Desktop (Mac/Windows) | [Claude Desktop setup](#claude-desktop) |
| Claude Code (command line) | [Claude Code setup](#claude-code) |
| Cursor | [Cursor setup](#cursor) |
| VS Code | [VS Code setup](#vs-code) |

---

### Claude Desktop

1. **Quit Claude Desktop** completely (not just close the window)
2. Open the config file:

   **Mac:** Open Terminal and run:
   ```bash
   open ~/Library/Application\ Support/Claude/claude_desktop_config.json
   ```

   **Windows:** Press Win+R, type this, and press Enter:
   ```
   notepad %APPDATA%\Claude\claude_desktop_config.json
   ```

3. Paste this configuration (replace the placeholders):

   **Mac:**
   ```json
   {
     "mcpServers": {
       "tieto-wiki": {
         "command": "/Users/YOUR-USERNAME/Downloads/mediawiki-mcp-server",
         "env": {
           "MEDIAWIKI_URL": "https://wiki.software-innovation.com/api.php",
           "MEDIAWIKI_USERNAME": "your.name@tietoevry.com#wiki-MCP",
           "MEDIAWIKI_PASSWORD": "paste-your-bot-password-here"
         }
       }
     }
   }
   ```

   **Windows:**
   ```json
   {
     "mcpServers": {
       "tieto-wiki": {
         "command": "C:\\Users\\YOUR-USERNAME\\Downloads\\mediawiki-mcp-server-windows.exe",
         "env": {
           "MEDIAWIKI_URL": "https://wiki.software-innovation.com/api.php",
           "MEDIAWIKI_USERNAME": "your.name@tietoevry.com#wiki-MCP",
           "MEDIAWIKI_PASSWORD": "paste-your-bot-password-here"
         }
       }
     }
   }
   ```

4. Replace:
   - `YOUR-USERNAME` with your computer username
   - `your.name@tietoevry.com` with your actual Tietoevry email
   - `paste-your-bot-password-here` with the bot password from Step 2

5. Save the file and reopen Claude Desktop

**How to find your username:**
- Mac: Open Terminal, type `whoami`, press Enter
- Windows: Open Command Prompt, type `echo %USERNAME%`, press Enter

---

### Claude Code

One command and you're done:

```bash
claude mcp add tieto-wiki ~/Downloads/mediawiki-mcp-server \
  -e MEDIAWIKI_URL="https://wiki.software-innovation.com/api.php" \
  -e MEDIAWIKI_USERNAME="your.name@tietoevry.com#wiki-MCP" \
  -e MEDIAWIKI_PASSWORD="paste-your-bot-password-here"
```

Replace the email and password with your actual credentials.

---

### Cursor

1. **Quit Cursor** completely
2. Open the MCP settings file:

   **Mac:** Open Terminal and run:
   ```bash
   open ~/Library/Application\ Support/Cursor/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json
   ```

   **Windows:** Press Win+R, type this, and press Enter:
   ```
   notepad %APPDATA%\Cursor\User\globalStorage\saoudrizwan.claude-dev\settings\cline_mcp_settings.json
   ```

   If the file doesn't exist, create it.

3. Paste this configuration:

   **Mac:**
   ```json
   {
     "mcpServers": {
       "tieto-wiki": {
         "command": "/Users/YOUR-USERNAME/Downloads/mediawiki-mcp-server",
         "env": {
           "MEDIAWIKI_URL": "https://wiki.software-innovation.com/api.php",
           "MEDIAWIKI_USERNAME": "your.name@tietoevry.com#wiki-MCP",
           "MEDIAWIKI_PASSWORD": "paste-your-bot-password-here"
         }
       }
     }
   }
   ```

   **Windows:**
   ```json
   {
     "mcpServers": {
       "tieto-wiki": {
         "command": "C:\\Users\\YOUR-USERNAME\\Downloads\\mediawiki-mcp-server-windows.exe",
         "env": {
           "MEDIAWIKI_URL": "https://wiki.software-innovation.com/api.php",
           "MEDIAWIKI_USERNAME": "your.name@tietoevry.com#wiki-MCP",
           "MEDIAWIKI_PASSWORD": "paste-your-bot-password-here"
         }
       }
     }
   }
   ```

4. Replace the placeholders with your actual values (same as Claude Desktop above)
5. Save the file and reopen Cursor

---

### VS Code

VS Code supports MCP servers through its built-in MCP configuration.

1. **Open VS Code**
2. Open the Command Palette: press **Ctrl+Shift+P** (Windows) or **Cmd+Shift+P** (Mac)
3. Type **"MCP: Add Server"** and select it
4. Choose **"Stdio"** as the transport type
5. When prompted for the command:

   **Mac:**
   ```
   /Users/YOUR-USERNAME/Downloads/mediawiki-mcp-server
   ```

   **Windows:**
   ```
   C:\Users\YOUR-USERNAME\Downloads\mediawiki-mcp-server-windows.exe
   ```

6. Give it the name: `tieto-wiki`

This creates a `.vscode/mcp.json` file. Open it and add the environment variables:

```json
{
  "servers": {
    "tieto-wiki": {
      "command": "/Users/YOUR-USERNAME/Downloads/mediawiki-mcp-server",
      "env": {
        "MEDIAWIKI_URL": "https://wiki.software-innovation.com/api.php",
        "MEDIAWIKI_USERNAME": "your.name@tietoevry.com#wiki-MCP",
        "MEDIAWIKI_PASSWORD": "paste-your-bot-password-here"
      }
    }
  }
}
```

On Windows, use the `.exe` path with double backslashes (`C:\\Users\\...`).

7. Replace the placeholders with your actual values
8. Save and reload VS Code (Ctrl+Shift+P, then "Developer: Reload Window")

**Note:** The wiki tools appear in Copilot Chat (the chat panel). Ask questions there and Copilot will use the wiki tools automatically.

---

## Step 4: Test the connection

1. Open your AI tool
2. Ask: **"Search the Tieto wiki for getting started"**
3. If it works, you'll see wiki search results!

**Claude Desktop:** Go to Settings (gear icon), then Developer. You should see "tieto-wiki" listed with a green status.

**VS Code:** Open Copilot Chat and look for the tools icon. You should see the wiki tools listed.

---

## Understanding what you just did

Here's what happened in simple terms:

- **The config file** is like a phonebook for your AI assistant. It tells the assistant where to find tools and how to use them.
- **MEDIAWIKI_URL** tells the tool where the wiki lives on the internet
- **MEDIAWIKI_USERNAME** and **MEDIAWIKI_PASSWORD** are your credentials. They prove you're allowed to access the wiki.
- **MCP** (Model Context Protocol) is the standard that lets AI assistants use external tools. It works the same way across Claude, Cursor, and VS Code.

---

## Troubleshooting

**Server not showing up**
- Make sure you completely quit and reopened the application (not just closed the window)
- Check that the config file is saved in the right location
- Make sure the JSON has no typos: matching quotes `"`, commas between items, no trailing commas

**"MEDIAWIKI_URL environment variable is required"**
- The config file format is wrong. Copy the example text again carefully.
- Make sure all quotation marks `"` are straight quotes, not curly quotes

**"authentication failed"**
- Check that your username is exactly: `your.name@tietoevry.com#wiki-MCP`
- Make sure you copied the bot password correctly (no extra spaces)
- The bot password might have expired. Create a new one at [Special:BotPasswords](https://wiki.software-innovation.com/wiki/Special:BotPasswords)

**"page does not exist"**
- Page titles are case-sensitive. Try: "resolve title [your search term]" to find the correct name.

**The file path doesn't work**
- Make sure you replaced `YOUR-USERNAME` with your actual computer username
- On Mac: Run `echo $HOME` in Terminal to find your home directory
- On Windows: Run `echo %USERPROFILE%` in Command Prompt

**Windows: "not recognized as an internal or external command"**
- Make sure the `.exe` file is in the path you specified
- Try using the full path: `C:\Users\YourName\Downloads\mediawiki-mcp-server-windows.exe`

**Still stuck?**
- Ask a colleague who has already set this up
- Check the main [README.md](README.md) for more detailed technical information
- Create an issue on [GitHub](https://github.com/olgasafonova/mediawiki-mcp-server/issues)

---

## What to try next

Now that you're connected, try asking your AI assistant:

**Search and discover:**
- "What does the wiki say about [topic]?"
- "Find all pages mentioning [keyword]"
- "Give me a quick overview of the [page name] page"
- "Get the content of [page A], [page B], and [page C]"

**Check quality:**
- "Are there broken links on the [page name] page?"
- "Find pages not updated in the last 90 days"
- "Check for outdated information on [page name]"

**Track changes:**
- "What pages were updated this week?"
- "Who edited [page name] recently?"

**Manage pages:**
- "Rename [old page name] to [new page name]"
- "Add category [name] to [page name]"

**Edit pages:**
- "Strike out [text] on [page name]"
- "Replace '[old text]' with '[new text]' on [page name]"

---

## Need help?

- **Main documentation:** [README.md](README.md)
- **Questions or issues:** [GitHub Issues](https://github.com/olgasafonova/mediawiki-mcp-server/issues)
- **More examples:** [WIKI_USE_CASES.md](WIKI_USE_CASES.md)
