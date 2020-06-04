package main

import (
	"context"
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
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

func main() {
	//Commands
	generateThumbsOnly := flag.Bool("thumbsonly", false, "Regenerates all thumbnails. You should run this if you change your thumbnail size or enable ffmpeg.")
	generatedHashesOnly := flag.Bool("dhashonly", false, "Regenerates all dhashes. You should run this if you change hash method, or after updating past 1.0.3.8")
	missingOnly := flag.Bool("missingonly", false, "When used with dhashonly or thumbsonly, prevents deleting pre-existing entries.")
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
		logging.LogInterface.WriteLog("Server", "Generate Thumbnails Flag", "*", "INFO", []string{"Generate thumbnails flag detected. Server will not start and instead just generate thumbnails. This may take some time."})
		//We need wait group so that we don't end the application before goroutines
		var wg sync.WaitGroup
		//list files
		files, err := ioutil.ReadDir(config.Configuration.ImageDirectory)
		if err != nil {
			logging.LogInterface.WriteLog("main", "generateThumbs", "CLI", "ERROR", []string{err.Error()})
			return
		}
		//for each image
		generatedThumbnails := uint64(0)
		for _, file := range files {
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
		}
		wg.Wait() //This will wait for all goroutines to finish
		logging.LogInterface.WriteLog("Server", "Generate Thumbnails Flag", "*", "SUCCESS", []string{"Finished generating " + strconv.FormatUint(generatedThumbnails, 10) + " new thumbnails."})
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
	//logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "Information", []string{fmt.Sprintf("%+v", config.Configuration)})
	if config.Configuration.DBName == "" || config.Configuration.DBPassword == "" || config.Configuration.DBUser == "" || config.Configuration.DBHost == "" {
		logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "Warning", []string{"Missing database information. (Instance, User, Password?)"})
	} else {
		//Initialize DB Connection
		database.DBInterface = &mariadbplugin.MariaDBPlugin{}
		err = database.DBInterface.InitDatabase()
		if err != nil {
			logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{"Failed to connect to database. Will keep trying. ", err.Error()})
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
				logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{"Error shutting down temp server. ", err.Error()})
			}
			cancel()
		}
		logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "INFO", []string{"Successfully connected to database"})
		configConfirmed = true
	}
	if *generatedHashesOnly {
		logging.LogInterface.WriteLog("Server", "Generate dHashes Flag", "*", "INFO", []string{"Generate dHashes flag detected. Server will not start and instead just generate dHashes. This will take some time."})
		//We need wait group so that we don't end the application before goroutines
		var wg sync.WaitGroup
		//for each image in the database
		page := uint64(0)
		processedImages := uint64(0)
		for true {
			images, maxCount, err := database.DBInterface.SearchImages([]interfaces.TagInformation{}, page, config.Configuration.PageStride)
			page += config.Configuration.PageStride
			if err != nil {
				logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{"Error processing hashes.", err.Error()})
				break
			}
			if len(images) <= 0 {
				logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "INFO", []string{"Finished queing images"})
				break
			}
			logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "INFO", []string{"Queing", strconv.FormatUint(page, 10), "of", strconv.FormatUint(maxCount, 10)})
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
		}
		logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "INFO", []string{"Waiting for images to finish processing"})
		wg.Wait() //This will wait for all goroutines to finish
		logging.LogInterface.WriteLog("Server", "Generate dHashes Flag", "*", "SUCCESS", []string{"Finished generating " + strconv.FormatUint(processedImages, 10) + " new dHashes."})

		return //We do not want to start server if used in cli
	}
	//Verify TLS Settings
	if config.Configuration.UseTLS {
		if _, err := os.Stat(config.Configuration.TLSCertPath); err != nil {
			configConfirmed = false
			logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{"Failed to stat TLS Cert file, does it exist? Does this application have permission to it?"})
		} else if _, err := os.Stat(config.Configuration.TLSKeyPath); err != nil {
			configConfirmed = false
			logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{"Failed to stat TLS Key file, does it exist? Does this application have permission to it?"})
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
		requestRouter.HandleFunc("/mod/user", routers.ModUserRouter)

		//API routers
		requestRouter.HandleFunc("/api/Collection/{CollectionID}", api.CollectionAPIRouter)
		requestRouter.HandleFunc("/api/Collections", api.CollectionsAPIRouter)
		//
		requestRouter.HandleFunc("/api/Tag/{TagID}", api.TagAPIRouter)
		requestRouter.HandleFunc("/api/Tags", api.TagsAPIRouter)
		//
		requestRouter.HandleFunc("/api/Image/{ImageID}", api.ImageAPIRouter)
		requestRouter.HandleFunc("/api/Images", api.ImagesAPIRouter)
		//
		requestRouter.HandleFunc("/api/Logon", api.LogonAPIRouter)
		requestRouter.HandleFunc("/api/Logout", api.LogoutAPIRouter)
		requestRouter.HandleFunc("/api/Users", api.UsersAPIRouter)
		//Autocomplete helpers
		requestRouter.HandleFunc("/api/TagName", api.TagNameAPIRouter)
		requestRouter.HandleFunc("/api/CollectionName", api.CollectionNameAPIRouter)

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
	if config.Configuration.UseTLS == false || configConfirmed == false {
		err = server.ListenAndServe()
	} else {
		logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "INFO", []string{"via tls"})
		err = server.ListenAndServeTLS(config.Configuration.TLSCertPath, config.Configuration.TLSKeyPath)
	}
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

func badConfigServerListenAndServe(serverEndedWG *sync.WaitGroup, server *http.Server) {
	defer serverEndedWG.Done()
	logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "INFO", []string{"Temp server now listening"})
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		logging.LogInterface.WriteLog("MAIN", "SERVER", "*", "ERROR", []string{"Error occured on temp server stop", err.Error()})
	}
}
