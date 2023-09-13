#!/bin/bash

if [ -f /tmp/.scaletest_failed ]; then
	echo "Failed"
elif [ -f /tmp/.scaletest_complete ]; then
	echo "Complete"
elif [ -f /tmp/.scaletest_running ]; then
	echo "Running"
elif [ -f /tmp/.scaletest_preparing ]; then
	echo "Preparing"
else
	echo "Not started"
fi
