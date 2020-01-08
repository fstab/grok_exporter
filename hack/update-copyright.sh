#!/bin/bash

find . -name '*.go' | while read file ; do
	year=$(git log -1 --pretty="format:%ci" "$file" | cut -f 1 -d -)
	sed -Ei "s,// Copyright ([0-9]{4})-[0-9]{4} ,// Copyright \\1-$year ," "$file"
done
