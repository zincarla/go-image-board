# Go! ImageBoard
Go! ImageBoard is a minimalistic booru style image board written in Go using a mysql/MariaDB backend with containerization as a primary goal.

![Font page example](http://ziviz.us/images/GIBFrontPage.PNG "Front Page")
----

![Example of search](http://ziviz.us/images/GIBSearch.PNG "Search Page")
----

![Example of image](http://ziviz.us/images/GIBIndividualImage.PNG "Individual Image Details")

## Installation
### Simple Docker Build
These steps will get you up and running immediatly
1. Copy the executable, http/*, and the dockerfile to your build directory
2. cd to your build directory
3. Build the image
```
docker build -t go-image-board .
```
4. Run a new instance of the imageboard

```
docker run --name myimageboard -p 80:8080 -v /var/docker/myimageboard/images:/var/go-image-board/images -v /var/docker/myimageboard/configuration:/var/go-image-board/configuration -d go-image-board
```
5. Stop the instance and edit the configuration file as needed
6. Start instance again

### Custom Docker Build
Similiar to the previous steps, the main difference here is that you are supplying your own template files to customize the look of the image board.
1. Copy the executable, http/*, and the dockerfile to your build directory
2. cd to your build directory
3. Build the image
```
docker build -t go-image-board .
```
4. Create a custom dockerfile that uses go-image-board as it's parent, and add your necessary changes

```
FROM go-image-board
COPY myhttp "/var/go-image-board/http"
WORKDIR /var/go-image-board
ENTRYPOINT ["/var/go-image-board/gib"]
```
5. Run a new instance of your imageboard

```
docker run --name myimageboard -p 80:8080 -v /var/docker/myimageboard/images:/var/go-image-board/images -v /var/docker/myimageboard/configuration:/var/go-image-board/configuration -d go-image-board
```
6. Stop the instance and edit the configuration file as needed
7. Start instance again

### TLS
Currently Go! ImageBoard does not natively support TLS. You can work around this however, by starting an Nginx reverse proxy container. This would also allow you to use [letsencrypt](https://letsencrypt.org/) and [certbot](https://certbot.eff.org/) to obtain a free trusted certificate that is relatively easy to install on Nginx.

### Configuration File
When you run Go! ImageBoard for the first time, the application will generate a new config file for you. This config file is JSON formatted and contains various configuration options. This file must be configured in order for Go! ImageBoard to be usable.

### Optional Darktheme
There is also an optional darktheme that can be enabled. To do so, edit /http/headerhtml and add
```
<link rel="stylesheet" href="/resources/darktheme.css">
```
under
```
<link rel="stylesheet" href="/resources/core.css">
```

![Example of DarkTheme](http://ziviz.us/images/GIBDarkSearch.PNG "DarkTheme Image Details")

## User Permissions
User permissions are stored in the database as an unsigned 64 bit integer where each bit represents a single permission flag. Since each bit is a single permission, you can add the permissions you want together to form your effective permissions.

## About files
Files located in the "/http/about/" directory are imported into the about.html template and served when requested from http://\<yourserver\>/about/\<filename\>.html
This can be used to easily write rules, or other documentation for your board while maintaining the same general theme.
