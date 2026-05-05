---
name: capture-demo
description: >-
  Capture screenshots or screen recordings from computer_use subagent
  sessions. Use when asked to record a demo, take a screenshot, save
  a recording, or document UI. Handles conversion, archiving to the
  recordings branch, and posting to GitHub PRs or Linear tickets.
---

# Capture Demo

Save screenshots or screen recordings from `computer_use` subagent
sessions to the `recordings` branch of `coder/coder`.

This skill is designed for
[Coder Agents](https://coder.com/docs/ai-coder/agents) with the
`computer_use` subagent type enabled. Coder Agents can spawn a
`computer_use` subagent to interact with a virtual desktop, and
the session is automatically recorded to an mp4. This skill covers
what to do after: converting to GIF, grabbing a good thumbnail or
screenshot, and archiving everything.

Requirements:
- A Coder Agents deployment with `computer_use` enabled (requires
  an Anthropic provider and the virtual desktop feature)
- `ffmpeg` installed in the workspace (the skill installs it if
  missing)
- Push access to the `recordings` branch

## When to use

- After a `computer_use` subagent finishes and you want to keep
  the recording or a screenshot
- "Record a demo", "take a screenshot", "save that recording",
  "document the UI"
- Preparing assets for a PR, issue, or Linear ticket

## Screenshots vs recordings

Not everything needs a full recording. If you just need a static
screenshot, spawn a `computer_use` subagent to navigate to the
right page, then grab the last frame from the recording. You get
the screenshot without needing to keep the full video.

For recordings, follow the full pipeline below. For screenshots
only, skip the GIF conversion (step 2) and just extract the
frame you want (step 3).

## Step 1: Find the recording

After a `computer_use` subagent completes, the recording lands
in `/tmp`:

```bash
find /tmp -name "coder-recording-*.mp4" -mmin -30
```

The filename is `coder-recording-<chat-id>.mp4`. A `.thumb.jpg`
may also exist but it's usually the first frame (blank desktop),
so ignore it.

## Step 2: Convert to GIF (recordings only)

Skip this for screenshots.

```bash
sudo apt-get install -y -qq ffmpeg 2>/dev/null

ffmpeg -y -i recording.mp4 \
  -vf "fps=10,scale=960:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse" \
  recording.gif
```

Expect roughly 2x the mp4 size. If over 5 MB, try `fps=8` or
`scale=720:-1`.

## Step 3: Extract a good frame

Don't use the auto `.thumb.jpg` from `/tmp`, it's almost always
a blank desktop.

Grab the last frame (usually shows the final result):

```bash
ffmpeg -y -sseof -1 -i recording.mp4 -frames:v 1 -update 1 thumbnail.jpg
```

Sanity check: >50 KB usually means real content, <20 KB is
probably blank. If you want to be sure, spawn a quick
`computer_use` subagent to eyeball it.

If the last frame isn't great, pull a few candidates:

```bash
DURATION=$(ffprobe -v quiet -show_entries format=duration -of csv=p=0 recording.mp4)
START=$(echo "$DURATION * 0.75" | bc)
ffmpeg -y -ss "$START" -i recording.mp4 -vf "fps=2" -frames:v 8 frame_%02d.jpg
```

## Step 4: Stage in /tmp

Get everything into a staging dir before touching git.

```bash
STAGING=/tmp/recording-stage-<feature-name>
mkdir -p "$STAGING"
cp recording.mp4 "$STAGING/"
cp recording.gif "$STAGING/"   # skip if screenshot only
cp thumbnail.jpg "$STAGING/"
```

Write a quick README:

```bash
cat > "$STAGING/README.md" << 'EOF'
# <Feature Name>

<1-2 sentence description.>

Recorded <date> against `<branch>` branch.

## What changed

- <bullet summary>

![Demo](recording.gif)
EOF
```

## Step 5: Push via worktree

Use a worktree so your working branch stays untouched.

```bash
cd /path/to/coder
git fetch origin recordings
git worktree add /tmp/recordings-worktree recordings
```

If the worktree already exists:

```bash
git worktree remove /tmp/recordings-worktree 2>/dev/null
git worktree add /tmp/recordings-worktree recordings
```

Copy, commit, push:

```bash
mkdir -p /tmp/recordings-worktree/recordings/<feature-name>
cp "$STAGING"/* /tmp/recordings-worktree/recordings/<feature-name>/

cd /tmp/recordings-worktree
git add recordings/       # NOT 'git add .'
git commit -m "recording: <feature-name>"
git push origin recordings
```

Clean up:

```bash
cd /path/to/coder
git worktree remove /tmp/recordings-worktree
```

## Step 6: Share it

### Where to use what

| Context | Format | Why |
|---|---|---|
| **GitHub PR/issue** | GIF embed | GitHub renders `![](url)` inline |
| **Linear ticket** | Plain links | Linear can't render image embeds or `<details>` |
| **Slack** | Thumbnail + mp4 link | GIFs autoplay and annoy people |
| **Archival** | All three | mp4 (lossless), GIF (preview), thumbnail (static) |

### GitHub

```markdown
![<feature> demo](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/recording.gif)

<details>
<summary>Full recording and thumbnail</summary>

- [mp4](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/recording.mp4)
- [Thumbnail](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/thumbnail.jpg)
</details>
```

### Linear

Linear can't render image embeds or `<details>`. Use plain links:

```markdown
- [Recording (GIF)](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/recording.gif)
- [Recording (mp4)](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/recording.mp4)
- [Thumbnail](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/thumbnail.jpg)
```

If the agent has Linear tools, post with `linear__save_comment`:

```
linear__save_comment(
  issueId: "TEAM-123",
  body: "## Demo\n\n- [Recording (GIF)](<url>)\n- [Recording (mp4)](<url>)\n- [Thumbnail](<url>)"
)
```

## Common issues

- **"moov atom not found"**: recording still being written. Wait
  for the subagent to fully finish.
- **GIF too large**: `fps=8` or `scale=720:-1`.
- **Blank thumbnail**: don't use the auto `.thumb.jpg`. Extract
  the last frame with `ffmpeg -sseof`.
- **Recording missing**: workspace may have restarted. Re-run the
  demo.
- **Lost files after branch switch**: use worktrees, not
  `git stash` + `git checkout`. Plain `git stash` drops untracked
  files.
- **"already checked out"**: `git worktree remove` the old one
  first.
