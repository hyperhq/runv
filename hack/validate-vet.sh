#!/bin/bash

files=( $(find . -path ./vendor -prune -o -name "*.go" -print) )

errors=()
for f in "${files[@]}"; do
	failedVet=$(go tool vet --printf=false "$f" 2>&1)
    if [ "$failedVet" ]; then
        errors+=( "$failedVet" )
    fi
done

if [ ${#errors[@]} -eq 0 ]; then
    echo 'Congratulations!  All Go source files have been vetted.'
else
    {
        echo "Errors from go vet:"
        for err in "${errors[@]}"; do
            echo " - $err"
        done
        echo
        echo 'Please fix the above errors. You can test via "go vet" and commit the result.'
        echo
    } >&2 
    false
fi
