//ToggleDIVDisplay is used to toggle a display from hidden to visible or vice versa. Used in showing user a message from the template input
function ToggleDIVDisplay(id) {
    if (document.getElementById(id).classList.contains("displayBlock") || document.getElementById(id).classList.contains("displayHidden") != true) {
        document.getElementById(id).classList.add("displayHidden");
        document.getElementById(id).classList.remove("displayBlock");
    } else {
        document.getElementById(id).classList.add("displayBlock");
        document.getElementById(id).classList.remove("displayHidden");
    }
}

//ToggleCellDIVDisplay is used to toggle a display from hidden to visible or vice versa while providing a default setting for use on phones.
function ToggleCellDIVDisplay(id) {
    if (document.getElementById(id).classList.contains("cellDefaultHidden")) {
        document.getElementById(id).classList.remove("cellDefaultHidden");
    } else {
        document.getElementById(id).classList.add("cellDefaultHidden");
    }
}

//ToggleFormDisplay is used to show hidden forms, cleans up the UI
function ToggleFormDisplay(id) {
    if (document.getElementById(id).classList.contains("displayHidden")) {
        document.getElementById(id).classList.remove("displayHidden");
    } else {
        document.getElementById(id).classList.add("displayHidden");
    }
    return false;
}

//toggleCSSClass is used to toggle a css class on a given html element.
function toggleCSSClass(elementID, className) {
    if (document.getElementById(elementID).classList.contains(className)) {
        document.getElementById(elementID).classList.remove(className);
    } else {
        document.getElementById(elementID).classList.add(className);
    }
}

//Expand Image or not
function ExpandImage() {
    parent = document.getElementById("ImageGridContainer");
    children = parent.getElementsByTagName("img");
    for (i = 0; i < children.length; i++) { 
        if (children[i].classList.contains("resetHeight")) {
            children[i].classList.remove("resetHeight");
        } else {
            children[i].classList.add("resetHeight");
        }
    }
    return false;
}

function UpdatePermissionBox() {
    var checkedBoxes = document.querySelectorAll('input[name=permCheckbox]:checked');
    var permValue = 0
    for (var I=0; I<checkedBoxes.length; I++) {
        permValue += Number(checkedBoxes[I].value);
    }
    document.querySelectorAll('input[name=permCheckbox]:checked');
    document.querySelector('input[name=permissions]').value = permValue;
}

//API
var CheckCollectionTimer = null;
function CheckCollectionName(form, resultID) {
    //document.getElementById(resultID).innerHTML = "Add to Collection"
    var xhttp = new XMLHttpRequest();
    xhttp.onreadystatechange = function() {
        if (this.readyState == 4 && this.status == 200 ) {
            //Get JSON object, and ID
            Result = JSON.parse(xhttp.responseText);
            if (userPermissions & 4096 == 4096 || userControlsOwn && (userID == Result.UploaderID)) {
                document.getElementById(resultID).innerHTML = "Add to Collection"
            } else {
                document.getElementById(resultID).innerHTML = "Insufficient Permissions!"
            }
        } else if (this.readyState == 4 && this.status == 404) {
            if (userPermissions & 2048) {
                document.getElementById(resultID).innerHTML = "Create Collection"
            } else {
                document.getElementById(resultID).innerHTML = "Insufficient Permissions!"
            }
        }
    };
    xhttp.open("GET", "/api/CollectionName?CollectionName="+form.elements["CollectionName"].value, true);
    xhttp.send();

    return false;
}

function SearchUsers(formID, pageStart) {
    form = document.getElementById(formID)
    var xhttp = new XMLHttpRequest();
    xhttp.onreadystatechange = function() {
        if (this.readyState == 4 && this.status == 200 ) {
            //Get JSON object, and ID
            Result = JSON.parse(xhttp.responseText);
            document.getElementById("userResultCount").innerText = Result.ResultCount+" users found!";
            //Clear Table
            table = document.getElementById("userResultTable")
            while (table.children.length > 1) {
                table.removeChild(table.children[1])
            }
            for (I = 0; I<Result.Users.length; I++) {
                newElem = document.createElement("tr")
                newElem.innerHTML =  "<td><a href=\"/mod/user?userName="+Result.Users[I].Name+"\">"+Result.Users[I].Name+"</a></td><td>"+Result.Users[I].ID+"</td><td>"+Result.Users[I].Permissions+"</td><td>"+Result.Users[I].Disabled+"</td><td>"+Result.Users[I].CreationTime+"</td>"
                table.appendChild(newElem);
            }
            GenerateUserSearchMenu(form.elements["userName"].value, Result.ServerStride, pageStart, Result.ResultCount, 'userResultPageMenu', formID)
        } else if (this.readyState == 4) {
            document.getElementById("userResultCount").innerText = "Error occurred: "+this.status+" - "+this.statusText+". From server: "+xhttp.responseText
        }
    };
    xhttp.open("GET", "/api/Users?userNameQuery="+form.elements["userName"].value+"&PageStart="+pageStart, true);
    xhttp.send();
    return false;
}

function GenerateUserSearchMenu(searchQuery, pageStride, pageOffset, maxCount, targetID, formID) {
    pageMenu = "<a href=\"#\" onclick=\"return SearchUsers('"+formID+"', 0);\">&#x3C;&#x3C;</a>"
    //Short circuit on param issues
    if (pageOffset < 0) {
        pageOffset = 0;
    }
    if (pageStride < 0 || maxCount <=0) {
        document.getElementById(targetID).innerHTML = "1";
        return false;
    }

    lastPage = Math.ceil(maxCount / pageStride);
    maxPage = lastPage;
    currentPage = Math.floor(pageOffset/pageStride) + 1;
    minPage = currentPage -3;
    if (minPage < 1) {
		minPage = 1;
	}
	if (maxPage > currentPage+3) {
		maxPage = currentPage + 3;
    }
    
    for (processPage = minPage; processPage <= maxPage; processPage++) {
        if (processPage != currentPage) {
            pageMenu = pageMenu + ", <a href=\"#\" onclick=\"return SearchUsers('"+formID+"', "+((processPage-1)*pageStride)+");\">"+processPage+"</a>";
        } else {
            pageMenu = pageMenu + ", "+processPage;
        }
    }

    pageMenu = pageMenu + ", <a href=\"#\" onclick=\"return SearchUsers('"+formID+"', "+((lastPage-1)*pageStride)+");\">&#x3E;&#x3E;</a>";

    document.getElementById(targetID).innerHTML = pageMenu;
}

//Helper functions for API
function GetImageType(path) {
    text = "";

    path = "."+path.toLowerCase().split('.').pop();

    switch(path) {
        case ".wav":
        case ".mp3":
        case ".ogg":
            text = "audio";
            break;
        case ".mpg":
        case ".mov":
        case ".webm":
        case ".avi":
        case ".mp4":
        case ".gif":
            text = "video";
            break;
        default:
            text = "image";
    } 

    return text;
}
function GetEmbedForContent(imageLocation) {
    ToReturn = ""
    
    ext = "."+imageLocation.toLowerCase().split('.').pop();
    console.log("Path for GetEmbedContent is "+imageLocation+" with extension of "+ext);

	switch(ext) {
        case ".jpg":
        case ".jpeg":
        case ".bmp":
        case ".gif":
        case ".png":
        case ".svg":
        case ".webp":
        case ".tiff":
        case ".tif":
        case ".jfif":
            ToReturn = "<img src=\"/images/" + imageLocation + "\" alt=\"" + imageLocation + "\" id=\"IMGContent\" />";
            break;
        case ".mpg":
        case ".mov":
        case ".webm":
        case ".avi":
        case ".mp4":
        case ".mp3":
        case ".ogg":
            ToReturn = "<video controls loop> <source src=\"/images/" + imageLocation + "\" type=\"" + getMIME(ext, "video/mp4") + "\">Your browser does not support the video tag.</video>";
            break;
        case ".wav":
            ToReturn = "<audio controls loop> <source src=\"/images/" + imageLocation + "\" type=\"" + getMIME(ext, "audio/wav") + "\">Your browser does not support the audio tag.</audio>";
            break;
        default:
            ToReturn = "<p>File format not supported. Click download.</p>";
    }

    console.log("Return for GetEmbedContent is "+ToReturn);
    
    wrapper = document.createElement('div');
    wrapper.innerHTML= ToReturn;

	return wrapper.firstChild;
}
function getMIME(extension, fallback) {
	switch (extension) {
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".avi":
		return "video/avi"
	case ".mpg":
		return "video/mpeg"
	case ".mov":
		return "video/quicktime"
	case ".ogg":
		return "video/ogg"
	case ".mp3":
		return "audio/mpeg3"
	case ".wav":
		return "audio/wav"
	default:
		return fallback
	}
}



function CorrectImageOrient(imgID) {
    var imgElement = document.getElementById(imgID);
     window.EXIF.getData(imgElement, function () {
        var orientation = EXIF.getTag(this, "Orientation");
        if (orientation && orientation != 1) {
            var canvas = window.loadImage.scale(imgElement, {orientation: orientation || 0,maxWidth: 1000,maxHeight: 1000,canvas:true});
            var newIMG = document.createElement("img");
            newIMG.src = canvas.toDataURL();
            newIMG.id = imgID;
            newIMG.alt = imgElement.alt;
            newIMG.style = imgElement.style;
            imgElement.parentNode.replaceChild(newIMG,imgElement);
        } //Otherwise not needed
    });
}

//Global Shortcuts
var MousetrapShortcuts = "?=help\r\nr=random search\r\nq=edit search terms\r\nright arrow=next image\r\nleft arrow=prev image\r\nctrl+right arrow=next page\r\nCtrl+left arrow=prev page";
Mousetrap.bind("?", function() { console.log(MousetrapShortcuts); });
Mousetrap.bind("r", function() {$("#mainImageSearchForm :input[value='Random']").click();})
Mousetrap.bind("q", function() {$("#mainImageSearchForm :input[name='SearchTerms']").select();$("#splashForm :input[name='SearchTerms']").select();})
Mousetrap.bind("right", function() {$("#mainImageSearchForm .nextInCollection").click();})
Mousetrap.bind("left", function() {$("#mainImageSearchForm .previousInCollection").click();})
Mousetrap.bind("ctrl+right", function() {$(".CollectionList .nextInCollection").click();})
Mousetrap.bind("ctrl+left", function() {$(".CollectionList .previousInCollection").click();})
Mousetrap.bind("shift+q", function() {$("#SideMenu form[action=\"/collections\"] :input[name='SearchTerms']").select();$("#SideMenu form[action=\"/tags\"] :input[name='SearchTags']").select();})