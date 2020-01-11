#!/bin/bash

find . -name '*.go' | while read file ; do
	year=$(git log -1 --pretty="format:%ci" "$file" | cut -f 1 -d -)
	sed -Ei "s,// Copyright ([0-9]{4})-[0-9]{4} ,// Copyright \\1-$year ," "$file" # range of years, like 2016-2020
	sed -Ei "/^\/\/ Copyright $year /! s,// Copyright ([0-9]{4}) ,// Copyright \\1-$year ," "$file" # single year but not current, like 2019
done

find . -type f -name '*.go' | while read file ; do
	if ! grep '// Copyright ' $file > /dev/null 2>&1 ; then
		echo "$file: missing copyright" >&2
	fi
done
