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

* **DBName**
Name of the database the imageboard should use
* **DBUser**
Account to use when communicating with the database
* **DBPassword**
Password to the account for communicating with the database
* **DBPort**
Port for the database (Default for MySQL is 3306)
* **DBHost**
Server name hosting the imageboard database
* **ImageDirectory**
If running in a container, this can be left at default. This is the directory that uploaded images should be saved to. Keep in mind, you should have a folder named "thumbs" in this one if you want the server to generate thumbnails. This is recommended as it will save bandwidth
* **Address**
Address the imageboard should listen for connections on. Default is :8080
* **ReadTimeout**
Read timeout for connections. Defaults are likely fine for this setting.
* **WriteTimout**
Write timeout for connections. Defaults are likely fine for this setting.
* **MaxHeaderBytes**
Maximum bytes allowed in a request header.
* **SessionStoreKey**
This is the key to your cookie store. This will be randomly set when you first run the imageboard
* **HTTPRoot**
Local directory containing the http files. You should not need to configure this if running in a container.
* **MaxUploadBytes**
Maximum file size in bytes that may be uploaded
* **AllowAccountCreation**
If true, anyone may make an account on your image board. Otherwise accounts would have to be created manually. If you want to be the only person allowed to upload images to your board, you could leave this to false.
* **MaxThumbnailWidth**
Maximum width for generated thumbnails. Smaller sizes will save bandwidth but progressively look worse, especially on mobile.
* **MaxThumbnailHeight**
Maximum height for generated thumbnails. Smaller sizes will save bandwidth but progressively look worse, especially on mobile.
* **DefaultPermissions**
Default permissions granted automatically to new accounts
* **UsersControlOwnObjects**
If true, the permission system is bypassed if a user is attempting to edit their own contributions.

### Optional Darktheme
There is also an optional darktheme that can be enabled. To do so, edit /http/headerhtml and add
```
<link rel="stylesheet" href="/resources/darktheme.css">
```
under
```
<link rel="stylesheet" href="/resources/core.css">
```

![Example of DarkTheme](http://ziviz.us/images/GIBDarkSearch.PNG "DarkThem Image Details")

## CLI tools

### -thumbsonly

Force the imageboard to regenerate *all* thumbnails. This could be a long running command, but is usefull if you change MaxThumbnailWidth or MaxThumbnailHeight.
```
gib -thumbsonly
```

## User Permissions
User permissions are stored in the database as an unsigned 64 bit interger where each bit represents a single permission flag. Below is a list of the current (2018-04-18) permission flags. Since each bit is a single permission, you can add the permissions you want together to form your effective permissions. 

For example, if you want to grant someone the ability to upload images (16) and modify tags applied to images (1), you add those two together to get 17. You have the option to allow users full control over their own contributions. If you set this in the configuration file, you could grant a user the upload permission (16), and the add tag permission (2). They could then add and tag their own images fully, without being able to edit other people's images or tags.
```
//ViewImagesAndTags View only access to Images and Tags
ViewImagesAndTags UserPermission = 0
//ModifyImageTags Allows a user to add and remove tags to/from an image, but not create or delete tags themselves
ModifyImageTags UserPermission = 1
//AddTags Allows a user to add a new tag to the system (But not delete)
AddTags UserPermission = 2
//ModifyTags Allows a user to modify a tag from the system
ModifyTags UserPermission = 4
//RemoveTags Allows a user to remove a tag from the system
RemoveTags UserPermission = 8
//UploadImage Allows a user to upload an image
UploadImage UserPermission = 16 //Note that it is probably a good idea to ensure users have ModifyImageTags at a minimum
//RemoveOtherImages Allows a user to remove an uploaded image. (Note that we can short circuit this in other code to allow a user to remove their own images)
RemoveImage UserPermission = 32
//DisableUser Allows a user to disable another user
DisableUser UserPermission = 64
//EditUserPermissions Allows a user to edit permissions of another user
EditUserPermissions UserPermission = 128
//BulkTagOperations
BulkTagOperations UserPermission = 256
```

## About files
Files located in the "/http/about/" directory are imported into the about.html template and served when requested from http://\<yourserver\>/about/\<filename\>.html
This can be used to easily write rules, or other documentation for your board while maintaining the same general theme.

## Programmer Reference
This section contains general notes on how to add onto the ImageBoard.

### Routers
General pattern to add a new router (Handler for a specific URL), is to create a new .go file under routers. This file should follow the pattern
```
package routers

import (
	"go-image-board/logging"
	"net/http"

	"github.com/gorilla/mux"
)

//ResourceRouter handles requests to /xxx
func XxxRouter(responseWriter http.ResponseWriter, request *http.Request) {
	urlVariables := mux.Vars(request)
	logging.LogInterface.WriteLog("ContentRouter", "GetCoreResource", "*", "SUCCESS", []string{"Someone accessed xxx"})
	//etc
}
```
Then activate the router function by adding it to gib.go's main() under the //Add routers comment
```
requestRouter.HandleFunc("/xxx", routers.XxxRouter)
```

### Images/Thumbnails
All functions that generate a thumbnail or serve an uploaded image are stored in resourcesrouters.go. To add a new image format to support, add it's handler to the import of resourcesrouters.go.
```
import (
...
_ "golang.org/x/image/webp"
...
)
```

### Add new database function
First update the dbinterface in dbinterface.go. Then implement the new interface in /plugins/mariadbplugin. That folder contains all the functions for directly communicating with the MariaDB. The files are named after and loosely contain functions based on which section of the database they interact with. MariaDBPlugin.go contains the database install/upgrade code and versioning. 

Since the database is based on an interface someone could program another database that implements the interface and replace MariaDB. After a new implementation of the interface is created, swap out the database in the main() function under the `//Initialize DB Connection` comment.

### Update Database Schema
1. In MariaDBPlugin.go, update the performFreshDBInstall() function to reflect new schema.
2. Increment database version number in performFreshDBInstall() as well, and the currentDBVersion variable in same file
3. Update InitDatabase() to add schema update code to upgrade a pre-existing database to the new schema
4. Update GetPluginInformation() to a new display version
5. If schema update code could not be added and schema changes are not backwards compatible, increment minSupportedDBVersion variable. This will prompt user action.

### To add a new metatag

Metatags come in 2 varieties. Plain and complex. These tags can also be limited to collection queries and image queries. 

For plain image metatags:
These queries tend to query the Images table. Things like score, rating, etc.
1. Update parseMetaTags() in TagQueryFunctions.go, note that the tag.Name should be the same name used in the database, not the name used in the query
As long as the limitations are adhered to, then this is the only change necessary outside of the schema changes.
2. To add to a form though so the value can be changed, you need to update ImageInformation struct and add a new property for the metatag
3. Update GetImage() function to return new information as part of ImageInformation object it returns
4. Edit Image.html template, and add a new form to change the metatag
5. Update database interface and add a function to change the column in the images table for the tag
6. Implement the new interface in MariaDBPlugin
7. Update the relevant router that the form will send the change request to

For complex image metatags:
1. Update parseMetaTags() in TagQueryFunctions.go. Mark the tag as complex and ensure the comparator used is valid. If you need to convert the value of the user input, ensure you do so. (ToAdd.MetaValue)
2. Update SearchImages in ImageSearchFunctions.go to support your new tag.

Context:
The parseMetaTags() function has a CollectionContext. This is set to true, if the user is querying for a collection. This can be used to validate certain tags that only work for one queries that only work either against images or collections.