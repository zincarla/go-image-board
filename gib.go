package main

import (
	"flag"
	"go-image-board/config"
	"go-image-board/database"
	"go-image-board/logging"
	"go-image-board/plugins"
	"go-image-board/plugins/mariadbplugin"
	"go-image-board/routers"
	"go-image-board/routers/api"
	"go-image-board/routers/templatecache"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

func main() {
	//Commands
	generateThumbsOnly := flag.Bool("thumbsonly", false, "Regenerates all thumbnails. You should run this if you change your thumbnail size or enable ffmpeg.")
	renameFilesOnly := flag.Bool("renameonly", false, "Renames all posts and corrects the names in the database. Use if changing naming convention of files.")
	flag.Parse()

	//Load succeeded
	configConfirmed := false
	//Init plugins
	logging.LogInterface = plugins.STDLog{}
	//Load Configuration
	configPath := "." + string(filepath.Separator) + "configuration" + string(filepath.Separator) + "config.json"
	err := config.LoadConfiguration(configPath)
	if err != nil {
		logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "Warning", []string{err.Error(), "Will use/save default file"})
	}
	//Add any missing configs
	fixMissingConfigs()
	if *generateThumbsOnly {
		//We need wait group so that we don't end the application before goroutines
		var wg sync.WaitGroup
		//list files
		files, err := ioutil.ReadDir(config.Configuration.ImageDirectory)
		if err != nil {
			logging.LogInterface.WriteLog("main", "generateThumbs", "CLI", "ERROR", []string{err.Error()})
			return
		}
		//for each image
		for _, file := range files {
			//Delete thumbnail
			os.Remove(config.Configuration.ImageDirectory + string(filepath.Separator) + "thumbs" + string(filepath.Separator) + file.Name() + ".png")
			//Goroutine generate a new one
			wg.Add(1) //This magic thing will prevent program from closing before goroutines finish
			go func(fileName string) {
				defer wg.Done()
				routers.GenerateThumbnail(fileName)
			}(file.Name())
		}
		wg.Wait() //This will wait for all goroutines to finish
		return    //We do not want to start server if used in cli
	}

	//Resave config file
	config.SaveConfiguration(configPath)

	//If we can, start the database
	//logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "Information", []string{fmt.Sprintf("%+v", config.Configuration)})
	if config.Configuration.DBName == "" || config.Configuration.DBPassword == "" || config.Configuration.DBUser == "" || config.Configuration.DBHost == "" {
		logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "Warning", []string{"Missing database information. (Instance, User, Password?)"})
	} else {
		//Initialize DB Connection
		database.DBInterface = &mariadbplugin.MariaDBPlugin{}
		err = database.DBInterface.InitDatabase()
		if err != nil {
			logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{"Failed to connect to database", err.Error()})
			database.DBInterface = nil
		} else {
			logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "INFO", []string{"Successfully connected to database"})
			configConfirmed = true
		}
	}
	//Init webserver cache
	templatecache.CacheTemplates()
	//Init API Throttle
	api.Throttle = api.ThrottleMap{}
	api.Throttle.Init()
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
		requestRouter.HandleFunc("/resources/{file}", routers.ResourceRouter)
		requestRouter.HandleFunc("/", routers.RootRouter)
		requestRouter.HandleFunc("/images", routers.ImageQueryRouter)
		requestRouter.HandleFunc("/collectionorder", routers.CollectionImageOrderRouter)
		requestRouter.HandleFunc("/collectionimages", routers.CollectionImageRouter)
		requestRouter.HandleFunc("/collections", routers.CollectionsRouter)
		requestRouter.HandleFunc("/images/{file}", routers.ResourceImageRouter)
		requestRouter.HandleFunc("/thumbs/{file}", routers.ThumbnailRouter)
		requestRouter.HandleFunc("/image", routers.ImageRouter)
		requestRouter.HandleFunc("/uploadImage", routers.UploadFormRouter)
		requestRouter.HandleFunc("/about/{file}", routers.AboutRouter)
		requestRouter.HandleFunc("/tags", routers.TagsRouter)
		requestRouter.HandleFunc("/tag", routers.TagRouter)
		requestRouter.HandleFunc("/redirect", routers.RedirectRouter)
		requestRouter.HandleFunc("/logon", routers.LogonRouter)
		requestRouter.HandleFunc("/mod", routers.ModRouter)
		//API routers
		requestRouter.HandleFunc("/api/Collection", api.CollectionAPIRouter)
		requestRouter.HandleFunc("/api/Collections", api.CollectionsAPIRouter)
		requestRouter.HandleFunc("/api/CollectionName", api.CollectionNameAPIRouter)
		//
		requestRouter.HandleFunc("/api/TagName", api.TagNameAPIRouter)
		requestRouter.HandleFunc("/api/Tag", api.TagAPIRouter)
		requestRouter.HandleFunc("/api/Tags", api.TagsAPIRouter)
		//
		requestRouter.HandleFunc("/api/Image", api.ImageAPIRouter)
		requestRouter.HandleFunc("/api/Images", api.ImagesAPIRouter)
		//
		requestRouter.HandleFunc("/api/Logon", api.LogonAPIRouter)
		requestRouter.HandleFunc("/api/Logout", api.LogoutAPIRouter)
		requestRouter.HandleFunc("/api/Users", api.UsersAPIRouter)

	} else {
		requestRouter.HandleFunc("/", routers.BadConfigRouter)
		requestRouter.HandleFunc("/resources/{file}", routers.ResourceRouter) /*Required for CSS*/
	}

	//Create server
	server := &http.Server{
		Handler:        requestRouter,
		Addr:           config.Configuration.Address,
		ReadTimeout:    config.Configuration.ReadTimeout,
		WriteTimeout:   config.Configuration.WriteTimeout,
		MaxHeaderBytes: config.Configuration.MaxHeaderBytes,
	}
	//Serve requests. Log on failure.
	logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "INFO", []string{"Server now listening"})
	err = server.ListenAndServe()
	if err != nil {
		logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{err.Error()})
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
