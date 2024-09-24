## golang setup
setup working directory in /var/www with permisssions for www-data
set up nginx with listen / directive at specified port
setup systemd service for restarts


## update
scp -r ./assets root@golang:/var/www/supchat/ ;
scp -r ./templates root@golang:/var/www/supchat/ ;
scp -r ./tests root@golang:/var/www/supchat/ ;
scp ./db.go root@golang:/var/www/supchat ;
scp ./hub.go root@golang:/var/www/supchat ;
scp ./go.mod root@golang:/var/www/supchat ;
scp ./main.go root@golang:/var/www/supchat ;
scp ./client.go root@golang:/var/www/supchat ;
scp ./go.sum root@golang:/var/www/supchat ;
