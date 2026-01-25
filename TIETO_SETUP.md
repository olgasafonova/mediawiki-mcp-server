# Connect Claude to Tieto's Public 360° wiki

This guide helps you connect Claude (your AI assistant) to Tieto's wiki at **wiki.software-innovation.com**. Don't worry if you're new to this - we'll walk through everything step by step!

---

## What you'll be able to do

Once connected, you can ask Claude things like:
- "What does the wiki say about eFormidling?"
- "Find all pages about AutoSaver"
- "Who edited the Release Notes last week?"
- "Are there broken links on the API documentation?"

---

## Before you start

You'll need:
1. **Claude Desktop** installed on your computer ([Download here](https://claude.ai/download))
2. **A Tieto account** to access the wiki
3. **10 minutes** of your time

---

## Step 1: Download the connection tool

Think of this as a bridge that lets Claude talk to the wiki.

### For Mac users:

1. Go to the [Releases page](https://github.com/olgasafonova/mediawiki-mcp-server/releases)
2. Download the file that says **macOS** (it will be named something like `mediawiki-mcp-server-darwin-amd64` or `mediawiki-mcp-server-darwin-arm64`)
3. Save it to your **Downloads** folder
4. Rename it to just `mediawiki-mcp-server` (remove the extra numbers and letters)
5. Open **Terminal** (you can find it by pressing Cmd+Space and typing "Terminal")
6. Type this command and press Enter:
   ```bash
   chmod +x ~/Downloads/mediawiki-mcp-server
   ```
   This makes the file usable by your computer.

### For Windows users:

1. Go to the [Releases page](https://github.com/olgasafonova/mediawiki-mcp-server/releases)
2. Download the file that says **Windows** (it will be named something like `mediawiki-mcp-server-windows-amd64.exe`)
3. Save it to your **Downloads** folder
4. You're done with this step!

---

## Step 2: Set up your wiki access

To edit pages on the wiki (optional - you can skip this if you only want to read), you need a special password.

### Creating a bot password (optional, only if you want to edit pages):

1. Open your web browser and go to: [https://wiki.software-innovation.com/wiki/Special:BotPasswords](https://wiki.software-innovation.com/wiki/Special:BotPasswords)
2. Log in with your Tieto account (your.email@tieto.com)
3. You'll see a page asking for a "Bot name" - type: **wiki-MCP**
4. Check these two boxes:
   - ✅ **Basic rights**
   - ✅ **Edit existing pages**
5. Click the **Create** button
6. **IMPORTANT:** You'll see a password - copy it and save it somewhere safe! You won't be able to see it again.
7. Your username will be: **your.email@tieto.com#wiki-MCP** (remember this!)

---

## Step 3: Connect Claude to the wiki

Now we'll tell Claude how to connect to the wiki. Don't worry about the technical terms - just follow along!

### For Mac users:

1. **Quit Claude Desktop** completely (click Claude in the menu bar, then Quit)
2. Open **Terminal** (press Cmd+Space, type "Terminal", press Enter)
3. Type this command and press Enter:
   ```bash
   open ~/Library/Application\ Support/Claude/claude_desktop_config.json
   ```
4. This opens a file that looks like a list of settings. It might be empty, or it might have some text in it.

5. **If the file is empty**, copy and paste this entire text:
   ```json
   {
     "mcpServers": {
       "tieto-wiki": {
         "command": "/Users/YOUR-USERNAME/Downloads/mediawiki-mcp-server",
         "env": {
           "MEDIAWIKI_URL": "https://wiki.software-innovation.com/api.php"
         }
       }
     }
   }
   ```

   **IMPORTANT:** Replace `YOUR-USERNAME` with your actual Mac username. To find it, type `whoami` in Terminal and press Enter - that's your username!

6. **If you created a bot password** (for editing), use this version instead:
   ```json
   {
     "mcpServers": {
       "tieto-wiki": {
         "command": "/Users/YOUR-USERNAME/Downloads/mediawiki-mcp-server",
         "env": {
           "MEDIAWIKI_URL": "https://wiki.software-innovation.com/api.php",
           "MEDIAWIKI_USERNAME": "your.email@tieto.com#wiki-MCP",
           "MEDIAWIKI_PASSWORD": "paste-your-bot-password-here"
         }
       }
     }
   }
   ```

   Replace:
   - `YOUR-USERNAME` with your Mac username
   - `your.email@tieto.com` with your actual Tieto email
   - `paste-your-bot-password-here` with the bot password you saved

7. Save the file (press Cmd+S) and close it
8. Open Claude Desktop again

### For Windows users:

1. **Quit Claude Desktop** completely (right-click Claude in the taskbar, click Quit)
2. Press **Windows key + R** on your keyboard
3. Type this and press Enter:
   ```
   %APPDATA%\Claude\claude_desktop_config.json
   ```
4. This opens a file that looks like a list of settings. It might be empty, or it might have some text in it.

5. **If the file is empty**, copy and paste this entire text:
   ```json
   {
     "mcpServers": {
       "tieto-wiki": {
         "command": "C:\\Users\\YOUR-USERNAME\\Downloads\\mediawiki-mcp-server.exe",
         "env": {
           "MEDIAWIKI_URL": "https://wiki.software-innovation.com/api.php"
         }
       }
     }
   }
   ```

   **IMPORTANT:** Replace `YOUR-USERNAME` with your actual Windows username. To find it, press Windows key + R, type `cmd`, press Enter, then type `echo %USERNAME%` and press Enter - that's your username!

6. **If you created a bot password** (for editing), use this version instead:
   ```json
   {
     "mcpServers": {
       "tieto-wiki": {
         "command": "C:\\Users\\YOUR-USERNAME\\Downloads\\mediawiki-mcp-server.exe",
         "env": {
           "MEDIAWIKI_URL": "https://wiki.software-innovation.com/api.php",
           "MEDIAWIKI_USERNAME": "your.email@tieto.com#wiki-MCP",
           "MEDIAWIKI_PASSWORD": "paste-your-bot-password-here"
         }
       }
     }
   }
   ```

   Replace:
   - `YOUR-USERNAME` with your Windows username
   - `your.email@tieto.com` with your actual Tieto email
   - `paste-your-bot-password-here` with the bot password you saved

7. Save the file (press Ctrl+S) and close it
8. Open Claude Desktop again

---

## Step 4: Test the connection

Let's make sure everything works!

1. Open Claude Desktop
2. Look for a small hammer icon (🔨) at the bottom of the chat window - this shows Claude has access to tools
3. Try asking Claude: **"Search the Tieto wiki for getting started"**
4. If it works, Claude will search the wiki and show you results!

---

## Understanding what you just did

Here's what happened in simple terms:

- **The config file** is like a phonebook for Claude - it tells Claude where to find tools and how to use them
- **JSON** is just a way of writing settings that computers can understand (those curly braces `{}` and quotes)
- **MEDIAWIKI_URL** tells the tool where the wiki lives on the internet
- **MEDIAWIKI_USERNAME** and **MEDIAWIKI_PASSWORD** are like your ID card - they prove you're allowed to edit pages

---

## Troubleshooting

**I don't see the hammer icon in Claude**
- Make sure you completely quit Claude and reopened it (not just closed the window)
- Check that the config file is saved in the right location
- Make sure the config file has no typos

**Claude says "MEDIAWIKI_URL environment variable is required"**
- Check that the config file has the correct format
- Make sure you have all the quotation marks `"` in the right places
- Try copying the example text again - sometimes invisible characters get in the way

**Claude says "authentication failed"**
- Check that your username is exactly: `your.email@tieto.com#wiki-MCP`
- Make sure you copied the bot password correctly (no extra spaces)
- The bot password might have expired - try creating a new one

**The file path doesn't work**
- Make sure you replaced `YOUR-USERNAME` with your actual username
- On Mac: Try typing `echo $HOME` in Terminal - use that path instead of `/Users/YOUR-USERNAME`
- On Windows: Try typing `echo %USERPROFILE%` in Command Prompt - use that path

**Still stuck?**
- Ask a colleague who has already set this up
- Check the main [README.md](README.md) for more detailed technical information
- Create an issue on [GitHub](https://github.com/olgasafonova/mediawiki-mcp-server/issues)

---

## What to try next

Now that you're connected, try asking Claude:

**Search and discover:**
- "What does the Tieto wiki say about [topic you're interested in]?"
- "Find all pages mentioning [keyword]"
- "Show me pages in the Documentation category"

**Check quality:**
- "Are there broken links on the [page name] page?"
- "Check for outdated information on [page name]"

**Track changes:**
- "What pages were updated this week?"
- "Who edited [page name] recently?"
- "Show me the history of [page name]"

**Edit pages (if you set up bot password):**
- "Strike out [text] on [page name]"
- "Make [text] bold on [page name]"
- "Replace '[old text]' with '[new text]' on [page name]"

---

## Need help?

- **Main documentation:** [README.md](README.md)
- **Questions or issues:** [GitHub Issues](https://github.com/olgasafonova/mediawiki-mcp-server/issues)
- **More examples:** [WIKI_USE_CASES.md](WIKI_USE_CASES.md)

---

**You're all set!** 🎉 Enjoy using Claude with the Tieto wiki.
