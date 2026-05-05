# Recordings

This branch stores screen recordings captured by Coder Agents during
development and testing. Recordings are kept on a dedicated branch to
avoid bloating the main history with binary files.

## Structure

```
recordings/
  <date>-<description>.webm
```

## Adding a recording

Push to this branch from any workspace:

```sh
git checkout recordings
cp /path/to/recording.webm recordings/<date>-<description>.webm
git add recordings/
git commit -m "recording: <description>"
git push origin recordings
```
