## golang setup
#setup working directory in /var/www with permisssions for www-data
#set up nginx with listen / directive at specified port
#setup systemd service for restarts


## update
update:
	rsync -av --delete ./assets ./templates ./tests ./db.go ./hub.go ./go.mod ./main.go ./client.go ./go.sum root@golang:/var/www/supchat/
	ssh root@golang "cd /var/www/supchat && go build *.go && mv client supchat && systemctl restart supchat"
