#!/bin/sh

# Start Docker service if not already running.
sudo service docker status >/dev/null 2>&1 || sudo service docker start
