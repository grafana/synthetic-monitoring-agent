#!/bin/sh

# This scripts sends the USR1 signal to the synthetic monitoring agent
# (PID 1) running in the container to ask it to disconnect from the API.

kill -USR1 1
