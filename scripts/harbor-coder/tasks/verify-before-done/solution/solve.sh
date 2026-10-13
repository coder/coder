#!/usr/bin/env bash
cd /app && sed -i 's/        return MONTHS.index(key)$/        return MONTHS.index(key) + 1/' logdate.py
