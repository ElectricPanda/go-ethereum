for i in `seq 1 64`; do
    export THREADNUM=$i
    echo ">>> test: threads=$THREADNUM"
    go test -v -run TestIMPT
done
