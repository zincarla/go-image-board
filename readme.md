# Go! ImageBoard

Go! ImageBoard is a minimalistic booru style image board written in Go using a mysql/MariaDB backend with containerization as a primary goal.

![Font page example](http://ziviz.us/images/GIBFrontPage.PNG "Front Page")
----

![Example of search](http://ziviz.us/images/GIBSearch.PNG "Search Page")
----

![Example of image](http://ziviz.us/images/GIBIndividualImage.PNG "Individual Image Details")

## Installation

You will need a functional MariaDB/MySQL instance for the service to use. If you plan to use docker, you will need a functional docker installation as well. You may run this without docker, however you will need to compile it yourself. Once you have the service installed, keep in mind how you are going to create your first admin account. See the `Your first account` section below for options.

### Simple Docker Build

These steps will get you up and running immediately

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

As of 1.0.3.3 Go! ImageBoard supports TLS. I still recommend using an Nginx reverse proxy container for its additional features and [letsencrypt](https://letsencrypt.org/) support. To enable TLS set UseTLS, TLSCertPath, and TLSKeyPath in your configuration file. Such as `{...,"UseTLS":true,"TLSCertPath":".\\configuration\\cert.pem","TLSKeyPath":".\\configuration\\server.key"}`

### Configuration File

When you run Go! ImageBoard for the first time, the application will generate a new config file for you. This config file is JSON formatted and contains various configuration options. This file must be configured in order for Go! ImageBoard to be usable and is located at `<InstallDirectory>/configuration/config.json`. A summary of settings can be found below:

Configuration Item | Description | Example | Default
--- | --- | --- | ---
DBName | is the name of the db used for this instance | `"myimageboard"` | `""` (No default, but required)
DBUser | is the user name used to auth to the db | `"myDBAccount"` | `""` (No default, but required)
DBPassword | is the password used to auth to the db | `"MySecretPWD"` | `""` (No default, but required)
DBPort | the port the database is listening to | `"3306"` | `""` (No default, but required)
DBHost | hostname of the database server | `"MyMariaDBServer"` | `""` (No default, but required)
ImageDirectory | path to where images are stored | `"/somepath/images"` | `"./images"`
Address | hostname/port that this server should listen on | `"myservername:80"` | `":8080"`
ReadTimeout | timeout allowed for reads | `60000000000` | `30000000000` (30 seconds)
WriteTimeout | timeout allowed for writes | `60000000000` | `30000000000` (30 seconds)
MaxHeaderBytes | maximum amount of bytes allowed in a request header | `2097152` | `1048576` (~1MiB)
SessionStoreKey | stores the key to the session store, saved as a pair of base64 binary | `["...","..."]` | A random 64 byte session store key is generated
CSRFKey | stores the master key for CSRF token, saved as binary in base64 | `"..."` | A random 32 byte key is generated
InSecureCSRF | marks wether CSRF cookie should be secure or not, when developing this may be set to true, otherwise, keep false! | `true` | `false`
HTTPRoot | directory where template and html files are kept | `"/somepath/http"` | `"./http"`
MaxUploadBytes | maximum allowed bytes for an upload | `209715200` | `104857600` (~100MiB)
AllowAccountCreation | if true, random users can create accounts, otherwise only mods can create users | `true` | `false`
AccountRequiredToView | if true, users must authenticate to access nearly any part of the server | `true` | `false`
MaxThumbnailWidth | Maximum width for automatically generated thumbnails | `804` | `402`
MaxThumbnailHeight | Maximum height for automatically generated thumbnails | `516` | `258`
DefaultPermissions | these permissions are assigned to all new users automatically | `24083` | `0`
UsersControlOwnObjects | if this is set, permission checks are ignored for users that are trying to manage resources they contributed | `true` | `false`
FFMPEGPath | Path to the FFMPEG application | `"./ffmpeg/ffmpeg.exe"` | `""`
UseFFMPEG | If set, when joined with FFMPEGPath, videos that are uploaded will have a thumbnail generated using FFMPEG | `true` | `false`
PageStride | How many images to show on one page | `60` | `30`
APIThrottle | How much time, in milliseconds, users using the API must wait between requests | `50` | `0`
UseTLS | Enables TLS encryption on server | `true` | `false`
TLSCertPath | The path to the TLS/SSL cert | `"./ssl/mycert.pem"` | `""`
TLSKeyPath | The path to the TLS/SSL key file for the cert | `"./ssl/mycert.key"` | `""`
ShowSimilarOnImages | If enabled, shows similar count and link when viewing an image | `true` | `false`
TargetLogLevel | increase or decrease log verbosity | `100` | `0` (See section below for log levels)
LoggingWhiteList | regex based white-list for logging | `".*FAIL.*"` | `""` (Empty string is ignored)
LoggingBlackList | regex based black-list for logging | `".*FAIL.*"` | `""` (Empty string is ignored)

#### Logging

Roughly, these are the log levels used. If you set your `TargetLogLevel` to a certain level, logs at that level, and below, are recorded.

Level | Name
--- | ---
0 | LogLevelCritical
10 | LogLevelError
20 | LogLevelWarning
30 | LogLevelInfo
40 | LogLevelDebug
50 | LogLevelVerbose

You may further restrict what is logged by setting the `LoggingWhiteList` and/or the `LoggingBlackList` configuration options. The regex is run on the whole logging line, which is usually in the format of:

`<CurrentTime> - <LogLevel> - <LogSource> - <RelatedUser> - <Result> - <Additional event-specific details, separated by more dashes>`

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

User permissions are stored in the database as an unsigned 64 bit integer where each bit represents a single permission flag. Since each bit is a single permission, you can add the permissions you want together to form your effective permissions. A good default for most people may be to set `DefaultPermissions` to `24087` and set `UsersControlOwnObjects` to `true`. This allows users to contribute, manage, and remove their own contributions, but does not allow them to delete resources from other users, or perform any administrative tasks. Once you have a board and admin created, you can explore the permissions in more depth under the Moderator tab.

### Your first account

When creating a new Go! Image Board, you have 2 options to create an admin account. 

The first, is to temporarily start your new board with `AllowAccountCreation` set to `true` and `DefaultPermissions` set to `4294967295`. Granting anyone who registers full admin. You can then create your account, set `DefaultPermissions` to something more reasonable and restart the service.

The second option, is to set `AllowAccountCreation` to `true`, create your account, and then manually set your permissions in the database to `4294967295`, granting your account full control.

## About files

Files located in the "/http/about/" directory are imported into the about.html template and served when requested from http://\<yourserver\>/about/\<filename\>.html
This can be used to easily write rules, or other documentation for your board while maintaining the same general theme.
