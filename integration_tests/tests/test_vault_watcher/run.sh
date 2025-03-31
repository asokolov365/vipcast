#!/bin/bash
# test_vault_watcher: runs an application container and 
# performs /bin/change-secret.sh -p secret/data/test -k foo -v bar to change watched file
set -e

# start up consul, vault and wait for leader election
docker-compose up -d consul vault
vault=$(docker-compose ps -q vault)
docker exec -it "$vault" assert ready
VAULT_ADDR=http://localhost:8200 VAULT_TOKEN=myroot ./change-secret.sh -p secret/data/test -k foo -v dieKaiw9od8j

docker-compose up -d app
sleep 10
VAULT_ADDR=http://localhost:8200 VAULT_TOKEN=myroot ./change-secret.sh -p secret/data/test -k foo -v waleo8ib3Zah
app=$(docker-compose ps -q app)
for i in $(seq 0 30); do
    sleep 1
    docker logs "$app" > app.log
    events=$(grep -c "event: {StatusChanged watch.secret/data/test}" app.log)
    receiver=$(grep -Ec "\"changed\!\" job=echo-when-secret-changed pid=" app.log)
    if [[ "$events" -eq 1 ]] && [[ "$receiver" -eq 1 ]]; then
        echo "secret change event emitted and received within $i seconds"
        break
    fi
done
if [ "$events" -ne 1 -o "$receiver" -ne 1 ]; then
    echo '--------------------'
    echo 'secret change event failed'
    echo '----- APP LOGS -----'
    cat app.log
    exit 1
fi
