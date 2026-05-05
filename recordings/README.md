# Recordings

Temporary screen recordings of Coder UI prototypes and features captured
by Coder Agents during development. Kept on a dedicated branch to avoid
bloating the main history with binary files.

## Structure

Each prototype or feature gets its own folder:

```
recordings/
  <feature-name>/
    recording.mp4    # full screen recording
    recording.gif    # animated GIF preview (embedded in PRs)
    thumbnail.jpg    # static frame showing the key result
    README.md        # what the recording shows
```

## Quick start

```sh
git fetch origin recordings
git checkout recordings
mkdir -p recordings/<feature-name>
# copy your files in (see agent skill for automated workflow)
git add recordings/
git commit -m "recording: <description>"
git push origin recordings
git checkout -  # back to your working branch
```
