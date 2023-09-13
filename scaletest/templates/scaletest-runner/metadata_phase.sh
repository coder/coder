#!/bin/bash

if [ -f /tmp/.scaletest_phase_creating_workspaces ]; then
	echo "Creating workspaces"
elif [ -f /tmp/.scaletest_phase_ssh ]; then
	echo "SSH traffic"
elif [ -f /tmp/.scaletest_phase_rpty ]; then
	echo "RPTY traffic"
elif [ -f /tmp/.scaletest_phase_dashboard ]; then
	echo "Dashboard traffic"
elif [ -f /tmp/.scaletest_phase_wait_baseline ]; then
	echo "Waiting $(</tmp/.scaletest_phase_wait_baseline)m (establishing baseline)"
else
	echo "None"
fi
