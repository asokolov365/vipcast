#!/bin/bash
# test_file_watcher: runs an application container and 
# perform /bin/change-file.sh -f /tmp/testFile.txt to change watched file

docker-compose up -d consul app
app=$(docker-compose ps -q app)
sleep 10
docker exec "$app" /bin/change-file.sh -f /tmp/testFile.txt
for i in $(seq 0 10); do
    sleep 1
    docker logs "$app" > app.log
    events=$(grep -c "event: {StatusChanged watch./tmp/testFile.txt}" app.log)
    receiver=$(grep -Ec "\"changed\!\" job=echo-when-file-changed pid=" app.log)
    if [[ "$events" -eq 1 ]] && [[ "$receiver" -eq 1 ]]; then
        echo "file change event emitted and received within $i seconds"
        break
    fi
done
if [[ "$events" -ne 1 ]] || [[ "$receiver" -ne 1 ]]; then
    echo '--------------------'
    echo 'file change event failed'
    echo '----- APP LOGS -----'
    cat app.log
    exit 1
fi
