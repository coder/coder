#!/usr/bin/env bash
cd /app && sed -i 's/if start < last_end:/if start <= last_end:/' intervals.py
