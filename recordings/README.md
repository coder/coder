# Recordings

Temporary screen recordings of Coder UI prototypes and features captured
by Coder Agents during development. Kept on a dedicated branch to avoid
bloating the main history with binary files.

## Structure

Each prototype or feature gets its own folder:

```
recordings/
  <feature-name>/
    recording.mp4    # screen recording
    recording.gif    # animated GIF preview
    thumbnail.jpg    # static thumbnail
    README.md        # what the recording shows
```

## Adding a recording

```sh
git fetch origin recordings
git checkout recordings
mkdir -p recordings/<feature-name>
cp /path/to/files recordings/<feature-name>/
git add recordings/
git commit -m "recording: <description>"
git push origin recordings
```
