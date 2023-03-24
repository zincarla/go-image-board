package main

import (
	"context"
	"database/sql"
	"flag"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"go-image-board/plugins"
	"go-image-board/plugins/mariadbplugin"
	"go-image-board/routers"
	"go-image-board/routers/api"
	"go-image-board/routers/templatecache"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
)

func main() {
	//Commands
	generateThumbsOnly := flag.Bool("thumbsonly", false, "Regenerates all thumbnails. You should run this if you change your thumbnail size or enable ffmpeg.")
	generatedHashesOnly := flag.Bool("dhashonly", false, "Regenerates all dhashes. You should run this if you change hash method, or after updating past 1.0.3.8")
	missingOnly := flag.Bool("missingonly", false, "When used with dhashonly or thumbsonly, prevents deleting pre-existing entries.")
	renameFilesOnly := flag.Bool("renameonly", false, "Renames all posts and corrects the names in the database. Use if changing naming convention of files.")
	removeOrphanFiles := flag.Bool("removeorphanfiles", false, "Removes images and thumbnails that do not have an associated database entry.")
	fixCollectionTags := flag.Bool("fixcollectiontags", false, "Validates and fixes tags applied to all collections")

	//For account creation
	newUserOnly := flag.Bool("createuser", false, "Creates a new user")
	newUserName := flag.String("username", "", "Name of your new user")
	newUserPassword := flag.String("password", "", "Password for your new user")
	newUserEmail := flag.String("email", "", "Email for your new user")
	newUserPermissions := flag.Uint64("permissions", 4294967295, "Permissions to grant new user (Defaults to admin!)")

	flag.Parse()

	//Load succeeded
	configConfirmed := false
	//Init plugins
	logging.LogInterface = &plugins.STDLog{}
	//Load Configuration
	configPath := "." + string(filepath.Separator) + "configuration" + string(filepath.Separator) + "config.json"
	err := config.LoadConfiguration(configPath)
	if err != nil {
		logging.WriteLog(logging.LogLevelWarning, "main/main", "0", logging.ResultFailure, []string{err.Error(), "Will use/save default file"})
	}
	//Add any missing configs
	fixMissingConfigs()

	//Init logging
	logging.LogInterface.Init(config.Configuration.TargetLogLevel, config.Configuration.LoggingWhiteList, config.Configuration.LoggingBlackList)

	if *generateThumbsOnly {
		logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultInfo, []string{"Generate thumbnails flag detected. Server will not start and instead just generate thumbnails. This may take some time."})
		//We need wait group so that we don't end the application before goroutines
		var wg sync.WaitGroup
		//list files
		files, err := ioutil.ReadDir(config.Configuration.ImageDirectory)
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"failed to get files to generate new thumbnails", err.Error()})
			return
		}
		//for each image
		generatedThumbnails := uint64(0)
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			//Delete thumbnail
			thumbNailPath := config.Configuration.ImageDirectory + string(filepath.Separator) + "thumbs" + string(filepath.Separator) + file.Name() + ".png"
			if _, err := os.Stat(thumbNailPath); *missingOnly == false || (err != nil && os.IsNotExist(err)) {
				os.Remove(thumbNailPath)
				//Goroutine generate a new one
				generatedThumbnails++
				wg.Add(1) //This magic thing will prevent program from closing before goroutines finish
				go func(fileName string) {
					defer wg.Done()
					routers.GenerateThumbnail(fileName)
				}(file.Name())
			}
			if generatedThumbnails%config.Configuration.PageStride == 0 {
				wg.Wait() //Throttle how fast we generate thumbnails
			}
		}
		wg.Wait() //This will wait for all goroutines to finish
		logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultSuccess, []string{"Finished generating " + strconv.FormatUint(generatedThumbnails, 10) + " new thumbnails."})
		return //We do not want to start server if used in cli
	}

	//Resave config file
	config.SaveConfiguration(configPath)

	//Init webserver cache
	templatecache.CacheTemplates()
	//Init API Throttle
	api.Throttle = api.ThrottleMap{}
	api.Throttle.Init()

	//If we can, start the database
	if config.Configuration.DBName == "" || config.Configuration.DBPassword == "" || config.Configuration.DBUser == "" || config.Configuration.DBHost == "" {
		logging.WriteLog(logging.LogLevelCritical, "main/main", "0", logging.ResultFailure, []string{"Missing database information. (Instance, User, Password?)"})
	} else {
		//Initialize DB Connection
		database.DBInterface = &mariadbplugin.MariaDBPlugin{}
		err = database.DBInterface.InitDatabase()
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"Failed to connect to database. Will keep trying. ", err.Error()})
			//Wait group for ending server
			serverEndedWG := &sync.WaitGroup{}
			serverEndedWG.Add(1)
			//Setup basic routers and server server
			requestRouter := mux.NewRouter()
			requestRouter.HandleFunc("/", routers.BadConfigRouter)
			requestRouter.HandleFunc("/resources/{file}", routers.ResourceRouter) /*Required for CSS*/
			server := &http.Server{
				Handler:        requestRouter,
				Addr:           config.Configuration.Address,
				ReadTimeout:    config.Configuration.ReadTimeout,
				WriteTimeout:   config.Configuration.WriteTimeout,
				MaxHeaderBytes: config.Configuration.MaxHeaderBytes,
			}
			//Actually start server listener in a goroutine
			go badConfigServerListenAndServe(serverEndedWG, server)
			//Now we loop for database connection
			for err != nil {
				time.Sleep(60 * time.Second) // retry interval
				err = database.DBInterface.InitDatabase()
			}
			//Kill server once we get a database connection
			waitCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			//Not defering cancel as this is the main function, instead calling it below after it is uneeded
			if err := server.Shutdown(waitCtx); err != nil {
				logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"Error shutting down temp server. ", err.Error()})
			}
			cancel()
		}
		logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultInfo, []string{"Successfully connected to database"})
		configConfirmed = true
	}
	if *generatedHashesOnly {
		logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultInfo, []string{"Generate dHashes flag detected. Server will not start and instead just generate dHashes. This will take some time."})
		//We need wait group so that we don't end the application before goroutines
		var wg sync.WaitGroup
		//for each image in the database
		page := uint64(0)
		processedImages := uint64(0)
		for true {
			images, maxCount, err := database.DBInterface.SearchImages([]interfaces.TagInformation{}, page, config.Configuration.PageStride)
			page += config.Configuration.PageStride
			if err != nil {
				logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"Error processing hashes.", err.Error()})
				break
			}
			if len(images) <= 0 {
				logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultInfo, []string{"Finished queing images"})
				break
			}
			logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultInfo, []string{"Queing", strconv.FormatUint(page, 10), "of", strconv.FormatUint(maxCount, 10)})
			for _, nextImage := range images {
				var dhashExists error
				if *missingOnly {
					_, _, dhashExists = database.DBInterface.GetImagedHash(nextImage.ID)
				}
				if *missingOnly == false || dhashExists != nil {
					processedImages++
					wg.Add(1) //This magic thing will prevent program from closing before goroutines finish
					go func(fileName string, imageID uint64) {
						defer wg.Done()
						routers.GeneratedHash(fileName, imageID)
					}(nextImage.Location, nextImage.ID)
				}
			}
			wg.Wait() //This will wait for all goroutines to finish
		}
		logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultInfo, []string{"Waiting for images to finish processing"})
		wg.Wait() //This will wait for all goroutines to finish
		logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultSuccess, []string{"Finished generating " + strconv.FormatUint(processedImages, 10) + " new dHashes."})

		return //We do not want to start server if used in cli
	}
	if *removeOrphanFiles {
		//Scan image directory
		files, err := ioutil.ReadDir(config.Configuration.ImageDirectory)
		if err != nil {
			logging.WriteLog(logging.LogLevelCritical, "main/main", "0", logging.ResultFailure, []string{"Failed to get images from directory", err.Error()})
			return
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			//Search database for matching image entry
			_, err := database.DBInterface.GetImageByFileName(file.Name())
			if err != nil && err == sql.ErrNoRows {
				logging.WriteLog(logging.LogLevelWarning, "main/main", "0", logging.ResultInfo, []string{"Failed to get image from database, it will be deleted", file.Name()})
				//If database entry does not exist, delete the image
				err = os.Remove(path.Join(config.Configuration.ImageDirectory, file.Name()))
				if err != nil {
					logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"Failed to delete image", file.Name(), err.Error()})
				}
			} else if err != nil {
				logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"Failed to get image from database due to an unexpected db error, it will be skipped", file.Name(), err.Error()})
			}
		}
		//Rinse&repeat with the thumbnails
		files, err = ioutil.ReadDir(path.Join(config.Configuration.ImageDirectory, "thumbs"))
		if err != nil {
			logging.WriteLog(logging.LogLevelCritical, "main/main", "0", logging.ResultFailure, []string{"Failed to get images from directory", err.Error()})
			return
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			//Search database for matching image entry
			imageName := file.Name()
			if len(imageName) > 4 { //Strip .png to get original name
				imageName = imageName[:len(imageName)-4]
			}
			_, err := database.DBInterface.GetImageByFileName(imageName)
			if err != nil && err == sql.ErrNoRows {
				logging.WriteLog(logging.LogLevelWarning, "main/main", "0", logging.ResultInfo, []string{"Failed to get image from database, it will be deleted", file.Name()})
				//If database entry does not exist, delete the image
				err = os.Remove(path.Join(path.Join(config.Configuration.ImageDirectory, "thumbs"), file.Name()))
				if err != nil {
					logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"Failed to delete thumbnail", file.Name(), err.Error()})
				}
			} else if err != nil {
				logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"Failed to get image from database due to an unexpected db error, it will be skipped", file.Name(), err.Error()})
			}
		}

		return //We do not want to start server if used in cli
	}
	if *fixCollectionTags {
		//Loop through all collections
		page := uint64(0)
		processedCollections := uint64(0)
		for true {
			collections, maxCount, err := database.DBInterface.SearchCollections([]interfaces.TagInformation{}, page, config.Configuration.PageStride)
			page += config.Configuration.PageStride
			if err != nil {
				logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"Error processing hashes.", err.Error()})
				break
			}
			if len(collections) <= 0 {
				logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultInfo, []string{"Finished queing collections"})
				break
			}
			logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultInfo, []string{"Queing", strconv.FormatUint(page, 10), "of", strconv.FormatUint(maxCount, 10)})
			for _, nextCollection := range collections {
				//Fix missing tags
				count, err := database.DBInterface.FixCollectionTags(nextCollection.ID)
				if err != nil {
					logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultInfo, []string{"Failed to fix collection images", err.Error()})
				} else if err == nil && count > 0 {
					logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultInfo, []string{"Fixed colllection tags", nextCollection.Name, "rows", strconv.FormatInt(count, 10)})
				}
				processedCollections++
			}
		}
		logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultInfo, []string{"Completed collection tag correction"})

		return //We do not want to start server if used in cli
	}
	if *newUserOnly {
		if *newUserName == "" || *newUserPassword == "" || *newUserEmail == "" {
			logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"When creating a new user by CLI, the Name, Password, and Email are required", err.Error()})
			return
		}
		if err = database.DBInterface.ValidateProposedUsername(*newUserName); err != nil {
			logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"Failed to validate username", *newUserName, err.Error()})
			return
		}
		if err = routers.ValidateProposedEmail(strings.ToLower(*newUserEmail)); err != nil {
			logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"Failed to validate user email", *newUserEmail, err.Error()})
			return
		}
		err = database.DBInterface.CreateUser(*newUserName, []byte(*newUserPassword), *newUserEmail, *newUserPermissions)
		if err != nil {
			logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"Failed to create requested user", *newUserName, *newUserEmail, strconv.FormatUint(*newUserPermissions, 10), err.Error()})
		}
		return
	}
	//Verify TLS Settings
	if config.Configuration.UseTLS {
		if _, err := os.Stat(config.Configuration.TLSCertPath); err != nil {
			configConfirmed = false
			logging.WriteLog(logging.LogLevelCritical, "main/main", "0", logging.ResultFailure, []string{"Failed to stat TLS Cert file, does it exist? Does this application have permission to it?"})
		} else if _, err := os.Stat(config.Configuration.TLSKeyPath); err != nil {
			configConfirmed = false
			logging.WriteLog(logging.LogLevelCritical, "main/main", "0", logging.ResultFailure, []string{"Failed to stat TLS Key file, does it exist? Does this application have permission to it?"})
		}
	}
	//Setup request routers
	requestRouter := mux.NewRouter()

	//Add router paths
	if configConfirmed == true {
		//Placing the rename function here, we need a validated connection to database for this to work
		if *renameFilesOnly {
			renameAllImages()
			return //We only wanted to rename
		}
		//Web routers
		requestRouter.HandleFunc("/resources/{file}", routers.ResourceRouter).Methods("GET")
		requestRouter.HandleFunc("/", routers.AccountRequiredMiddleWare(routers.RootRouter)).Methods("GET")
		requestRouter.HandleFunc("/images", routers.AccountRequiredMiddleWare(routers.ImageQueryRouter)).Methods("GET")
		requestRouter.HandleFunc("/collectionorder", routers.AccountRequiredMiddleWare(routers.CollectionImageOrderGetRouter)).Methods("GET")
		requestRouter.HandleFunc("/collectionorder", routers.AccountRequiredMiddleWare(routers.CollectionImageOrderPostRouter)).Methods("POST")
		requestRouter.HandleFunc("/collection", routers.AccountRequiredMiddleWare(routers.CollectionGetRouter)).Methods("GET")
		requestRouter.HandleFunc("/collection", routers.AccountRequiredMiddleWare(routers.CollectionPostRouter)).Methods("POST")
		requestRouter.HandleFunc("/collections", routers.AccountRequiredMiddleWare(routers.CollectionsRouter)).Methods("GET")
		requestRouter.HandleFunc("/images/{file}", routers.AccountRequiredMiddleWare(routers.ResourceImageRouter)).Methods("GET")
		requestRouter.HandleFunc("/thumbs/{file}", routers.AccountRequiredMiddleWare(routers.ThumbnailRouter)).Methods("GET")
		requestRouter.HandleFunc("/image", routers.AccountRequiredMiddleWare(routers.ImageGetRouter)).Methods("GET")
		requestRouter.HandleFunc("/image", routers.AccountRequiredMiddleWare(routers.ImagePostRouter)).Methods("POST")
		requestRouter.HandleFunc("/uploadImage", routers.AccountRequiredMiddleWare(routers.UploadFormRouter)).Methods("GET")
		requestRouter.HandleFunc("/about/{file}", routers.AccountRequiredMiddleWare(routers.AboutRouter)).Methods("GET")
		requestRouter.HandleFunc("/tags", routers.AccountRequiredMiddleWare(routers.TagsRouter)).Methods("GET")
		requestRouter.HandleFunc("/tag", routers.AccountRequiredMiddleWare(routers.TagGetRouter)).Methods("GET")
		requestRouter.HandleFunc("/tag", routers.AccountRequiredMiddleWare(routers.TagPostRouter)).Methods("POST")
		requestRouter.HandleFunc("/redirect", routers.AccountRequiredMiddleWare(routers.RedirectRouter)).Methods("POST")
		requestRouter.HandleFunc("/logon", routers.LogonGetRouter).Methods("GET")
		requestRouter.HandleFunc("/logon", routers.LogonPostRouter).Methods("POST")
		requestRouter.HandleFunc("/mod", routers.AccountRequiredMiddleWare(routers.ModRouter)).Methods("GET")
		requestRouter.HandleFunc("/mod/user", routers.AccountRequiredMiddleWare(routers.ModUserGetRouter)).Methods("GET")
		requestRouter.HandleFunc("/mod/user", routers.AccountRequiredMiddleWare(routers.ModUserPostRouter)).Methods("POST")

		//API routers
		requestRouter.HandleFunc("/api/Collection/{CollectionID}", api.CollectionGetAPIRouter).Methods("GET")
		requestRouter.HandleFunc("/api/Collection/{CollectionID}", api.CollectionDeleteAPIRouter).Methods("DELETE")
		requestRouter.HandleFunc("/api/Collections", api.CollectionsGetAPIRouter).Methods("GET")
		//
		requestRouter.HandleFunc("/api/Tag/{TagID}", api.TagGetAPIRouter).Methods("GET")
		requestRouter.HandleFunc("/api/Tag/{TagID}", api.TagDeleteAPIRouter).Methods("DELETE")
		requestRouter.HandleFunc("/api/Tags", api.TagsGetAPIRouter).Methods("GET")
		//
		requestRouter.HandleFunc("/api/Image/{ImageID}", api.ImageGetAPIRouter).Methods("GET")
		requestRouter.HandleFunc("/api/Image/{ImageID}", api.ImageDeleteAPIRouter).Methods("DELETE")
		requestRouter.HandleFunc("/api/Image", api.ImagePostAPIRouter).Methods("POST")
		requestRouter.HandleFunc("/api/Images", api.ImagesGetAPIRouter).Methods("GET")
		//
		requestRouter.HandleFunc("/api/Image/{ImageID}/Tags", api.ImageTagsGetAPIRouter).Methods("GET")
		requestRouter.HandleFunc("/api/Image/{ImageID}/Tags/{TagID}", api.ImageTagsDeleteAPIRouter).Methods("DELETE")
		requestRouter.HandleFunc("/api/Image/{ImageID}/Tags", api.ImageTagsPostAPIRouter).Methods("POST")
		//
		requestRouter.HandleFunc("/api/Logon", api.LogonAPIRouter).Methods("POST")
		requestRouter.HandleFunc("/api/Logout", api.LogoutAPIRouter).Methods("POST")
		requestRouter.HandleFunc("/api/Users", api.UsersAPIRouter).Methods("GET")
		//Autocomplete helpers
		requestRouter.HandleFunc("/api/TagName", api.TagNameAPIRouter).Methods("GET")
		requestRouter.HandleFunc("/api/CollectionName", api.CollectionNameAPIRouter).Methods("GET")
		requestRouter.HandleFunc("/api", api.CSRFAPIRouter).Methods("GET")

	} else {
		requestRouter.HandleFunc("/", routers.BadConfigRouter).Methods("GET")
		requestRouter.HandleFunc("/resources/{file}", routers.ResourceRouter).Methods("GET") /*Required for CSS*/
	}

	requestRouter.Use(routers.LogMiddleware)

	//Setup csrf protected routers
	csrfRequestRouter := csrf.Protect(config.Configuration.CSRFKey, csrf.Secure(!config.Configuration.InSecureCSRF),
		csrf.ErrorHandler(http.HandlerFunc(routers.CSRFErrorRouter)))(requestRouter)

	//Create server
	server := &http.Server{
		Handler:        csrfRequestRouter,
		Addr:           config.Configuration.Address,
		ReadTimeout:    config.Configuration.ReadTimeout,
		WriteTimeout:   config.Configuration.WriteTimeout,
		MaxHeaderBytes: config.Configuration.MaxHeaderBytes,
	}
	//Serve requests. Log on failure.
	logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultInfo, []string{"Server now listening"})
	if config.Configuration.UseTLS == false || configConfirmed == false {
		err = server.ListenAndServe()
	} else {
		logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultInfo, []string{"via tls"})
		err = server.ListenAndServeTLS(config.Configuration.TLSCertPath, config.Configuration.TLSKeyPath)
	}
	if err != nil {
		logging.WriteLog(logging.LogLevelCritical, "main/main", "0", logging.ResultFailure, []string{err.Error()})
	}
}

func fixMissingConfigs() {
	if config.Configuration.Address == "" {
		config.Configuration.Address = ":8080"
	}
	if config.Configuration.ImageDirectory == "" {
		config.Configuration.ImageDirectory = "." + string(filepath.Separator) + "images"
	}
	if config.Configuration.HTTPRoot == "" {
		config.Configuration.HTTPRoot = "." + string(filepath.Separator) + "http"
	}
	if config.Configuration.MaxUploadBytes <= 0 {
		config.Configuration.MaxUploadBytes = 100 << 20
	}
	if config.Configuration.MaxHeaderBytes <= 0 {
		config.Configuration.MaxHeaderBytes = 1 << 20
	}
	if config.Configuration.ReadTimeout.Nanoseconds() <= 0 {
		config.Configuration.ReadTimeout = 30 * time.Second
	}
	if config.Configuration.WriteTimeout.Nanoseconds() <= 0 {
		config.Configuration.WriteTimeout = 30 * time.Second
	}
	if config.Configuration.MaxThumbnailWidth <= 0 {
		config.Configuration.MaxThumbnailWidth = 402
	}
	if config.Configuration.MaxThumbnailHeight <= 0 {
		config.Configuration.MaxThumbnailHeight = 258
	}
	if config.Configuration.PageStride <= 0 {
		config.Configuration.PageStride = 30
	}
	config.CreateSessionStore()
}

func badConfigServerListenAndServe(serverEndedWG *sync.WaitGroup, server *http.Server) {
	defer serverEndedWG.Done()
	logging.WriteLog(logging.LogLevelInfo, "main/main", "0", logging.ResultInfo, []string{"Temp server now listening"})
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		logging.WriteLog(logging.LogLevelError, "main/main", "0", logging.ResultFailure, []string{"Error occured on temp server stop", err.Error()})
	}
}
