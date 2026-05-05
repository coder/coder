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

## Step 4: Push to the recordings branch

```bash
cd /path/to/coder
git stash                 # save current work
git fetch origin recordings
git checkout recordings

mkdir -p recordings/<feature-name>
cp recording.mp4 recordings/<feature-name>/recording.mp4
cp recording.gif  recordings/<feature-name>/recording.gif
cp thumbnail.jpg  recordings/<feature-name>/thumbnail.jpg
```

Write a README for the recording folder:

```markdown
# <Feature Name>

<1-2 sentence description of what this demonstrates.>

Recorded <date> against `<branch>` branch.

## What changed

- <bullet summary of the changes being demoed>

![Demo](recording.gif)
```

Commit and push:

```bash
git add recordings/
git commit -m "recording: <feature-name>"
git push origin recordings
git checkout -            # back to working branch
git stash pop
```

## Step 5: Embed in PRs and issues

### Which format to use where

| Context | Embed | Why |
|---|---|---|
| **PR description** | GIF | Renders inline, reviewers see it immediately |
| **GitHub issue** | GIF | Same reason |
| **Slack / async chat** | Thumbnail + mp4 link | GIFs autoplay and annoy people in chat |
| **Archival / repo** | All three | Keep mp4 (lossless), GIF (preview), thumbnail (static reference) |

### Embedding pattern

Always upload all three files to the recordings branch, then embed
the GIF and link the others:

```markdown
## Demo

![<feature> demo](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/recording.gif)

<details>
<summary>Full recording and static thumbnail</summary>

- [Full recording (mp4)](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/recording.mp4)
- [Thumbnail](https://raw.githubusercontent.com/coder/coder/recordings/recordings/<feature>/thumbnail.jpg)
</details>
```

This keeps the PR body clean (just the GIF) while making the mp4
and thumbnail available for anyone who needs them.

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
