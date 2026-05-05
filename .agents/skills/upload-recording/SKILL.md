---
name: upload-recording
description: >-
  Find, process, and upload screen recordings from computer_use subagent
  sessions. Use when asked to save a recording, create a demo, or
  document UI with a screen capture. Converts to GIF, extracts a
  meaningful thumbnail, and pushes to the coder/coder recordings branch.
---

# Upload Recording

Capture, convert, and archive screen recordings from `computer_use`
subagent sessions to the `recordings` branch of `coder/coder`.

This skill is designed for
[Coder Agents](https://coder.com/docs/ai-coder/agents) with the
`computer_use` subagent type enabled. Coder Agents can spawn a
`computer_use` subagent to interact with a virtual desktop, and
the session is automatically recorded to an mp4 file. This skill
handles everything after the recording is captured: converting to
GIF, extracting a thumbnail, and archiving to the recordings
branch.

Requirements:
- A Coder Agents deployment with `computer_use` enabled (requires
  an Anthropic provider and the virtual desktop feature)
- `ffmpeg` installed in the workspace (the skill installs it if
  missing)
- Push access to the `recordings` branch

## When to use

- After a `computer_use` subagent finishes and you need to save
  its recording
- When asked to "record a demo", "save that recording", or
  "document the UI"
- When preparing assets for a PR description or issue

## Step 1: Find the recording

After a `computer_use` subagent completes, its response includes a
`recording_file_id`. The actual file lands in `/tmp`:

```bash
find /tmp -name "coder-recording-*.mp4" -mmin -30
```

The filename pattern is `coder-recording-<chat-id>.mp4` where
`<chat-id>` matches the subagent's chat ID. A `.thumb.jpg` may
also exist but it is usually just the first frame (often a blank
desktop), so do not use it as the final thumbnail.

## Step 2: Convert to GIF

Install ffmpeg if missing, then convert:

```bash
sudo apt-get install -y -qq ffmpeg 2>/dev/null

ffmpeg -y -i recording.mp4 \
  -vf "fps=10,scale=960:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse" \
  recording.gif
```

Expect roughly 2x the mp4 size. If the GIF exceeds 5 MB, reduce
to `fps=8` or `scale=720:-1`.

## Step 3: Extract a meaningful thumbnail

Do NOT use the auto-generated `.thumb.jpg` from `/tmp`. It captures
the first frame which is usually a blank desktop or loading state.

Extract the last frame of the video (where the final result is
visible):

```bash
ffmpeg -y -sseof -1 -i recording.mp4 -frames:v 1 -update 1 thumbnail.jpg
```

**Verify the thumbnail** by spawning a quick `computer_use` subagent
to open it in an image viewer, or at minimum check that the file
size is reasonable (>50 KB usually means real content, <20 KB is
likely a blank screen).

If the last frame is not ideal (e.g. a transition), extract a few
candidate frames from the last quarter of the video and pick the
best one:

```bash
DURATION=$(ffprobe -v quiet -show_entries format=duration -of csv=p=0 recording.mp4)
START=$(echo "$DURATION * 0.75" | bc)
ffmpeg -y -ss "$START" -i recording.mp4 -vf "fps=2" -frames:v 8 frame_%02d.jpg
```

## Step 4: Stage files in /tmp

Copy all processed files to a staging directory before touching git.

```bash
STAGING=/tmp/recording-stage-<feature-name>
mkdir -p "$STAGING"
cp recording.mp4 "$STAGING/"
cp recording.gif "$STAGING/"
cp thumbnail.jpg "$STAGING/"
```

Write the per-recording README in the staging dir too:

```bash
cat > "$STAGING/README.md" << 'EOF'
# <Feature Name>

<1-2 sentence description of what this demonstrates.>

Recorded <date> against `<branch>` branch.

## What changed

- <bullet summary of the changes being demoed>

![Demo](recording.gif)
EOF
```

## Step 5: Push to the recordings branch via worktree

Use `git worktree` to check out the recordings branch in a
separate directory. This avoids stashing, branch switching, or
disrupting your working branch in any way.

```bash
cd /path/to/coder
git fetch origin recordings
git worktree add /tmp/recordings-worktree recordings
```

Copy from the staging directory and commit:

```bash
mkdir -p /tmp/recordings-worktree/recordings/<feature-name>
cp "$STAGING"/* /tmp/recordings-worktree/recordings/<feature-name>/

cd /tmp/recordings-worktree
git add recordings/
git status                # verify only recordings/ files are staged
git commit -m "recording: <feature-name>"
git push origin recordings
```

Clean up the worktree when done:

```bash
cd /path/to/coder
git worktree remove /tmp/recordings-worktree
```

Your working branch is completely untouched throughout this process.

**If the worktree already exists** (e.g. from a previous run):

```bash
git worktree remove /tmp/recordings-worktree 2>/dev/null
git worktree add /tmp/recordings-worktree recordings
```

## Step 6: Embed in PRs, issues, and Linear tickets

### Which format to use where

| Context | Embed | Why |
|---|---|---|
| **PR description** | GIF embed | GitHub renders `![](url)` inline |
| **GitHub issue** | GIF embed | Same reason |
| **Linear ticket** | Plain links | Linear cannot render image embeds or `<details>` tags |
| **Slack / async chat** | Thumbnail + mp4 link | GIFs autoplay and annoy people in chat |
| **Archival / repo** | All three | Keep mp4 (lossless), GIF (preview), thumbnail (static reference) |

### GitHub (PRs and issues)

GitHub renders `![](url)` image embeds and `<details>` tags, so
use an inline GIF with collapsible mp4/thumbnail links:

```markdown
## Demo

![<feature> demo](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/recording.gif)

<details>
<summary>Full recording and static thumbnail</summary>

- [Full recording (mp4)](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/recording.mp4)
- [Thumbnail](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/thumbnail.jpg)
</details>
```

### Linear tickets

Linear does NOT render image embeds (`![](url)` shows as broken)
and does NOT support `<details>` HTML tags. Use plain markdown
links only:

```markdown
## Demo: <Feature Name>

- [Recording (GIF)](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/recording.gif)
- [Recording (mp4)](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/recording.mp4)
- [Thumbnail](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/thumbnail.jpg)
```

If the Coder Agent has Linear tools, post directly with
`linear__save_comment`:

```
linear__save_comment(
  issueId: "TEAM-123",
  body: "## Demo: <Feature>\n\n- [Recording (GIF)](<gif-url>)\n- [Recording (mp4)](<mp4-url>)\n- [Thumbnail](<thumb-url>)"
)
```

## Common issues

- **"moov atom not found"** when probing the mp4: the file was
  copied while still being written. Wait for the subagent to fully
  complete, or use the `/tmp` copy which finalizes on session end.
- **GIF too large**: reduce fps (`fps=8`) or width (`scale=720:-1`).
- **Blank thumbnail**: you used the auto `.thumb.jpg`. Always
  extract the last frame yourself with `ffmpeg -sseof`.
- **Recording not in /tmp**: the workspace may have restarted. Ask
  the user to re-run the demo. Recordings do not persist across
  workspace restarts.
- **Lost untracked files after branch switch**: if you used
  `git checkout` instead of worktrees, plain `git stash` drops
  untracked files. New files you just created will be gone. Use
  worktrees to avoid this entirely.
- **Stray files committed to wrong branch**: untracked files
  persist in the working tree across `git checkout`. This cannot
  happen with worktrees since each worktree has its own directory.
- **"fatal: '<branch>' is already checked out"**: you already have
  a worktree for that branch. Remove it first with
  `git worktree remove /tmp/recordings-worktree`.
- **Skills and code do not go on the recordings branch**: the
  recordings branch is for binary assets and their READMEs. This
  skill file in `.skills/` is the one exception since it documents
  the branch's own workflow.
