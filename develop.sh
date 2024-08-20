#!/bin/sh
# This script sets up the development environment by creating necessary directories and a .env file.
mkdir -p logs database
echo "LOG_DIR='./logs'
DB_DIR='./database'
ENDPOINT='localhost:8000'" > .env