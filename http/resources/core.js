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

//
function SearchTagTable(tagTableID, formID) {
    parent = document.getElementById(tagTableID);
    query = document.getElementById(formID).query.value.toLowerCase();
    children = parent.getElementsByTagName("tr");
    for (i = 0; i < children.length; i++) { 
        if (children[i].innerText.toLowerCase().includes(query)) {
            if (children[i].classList.contains("displayHidden") && children[i].innerText.includes(query)) {
                children[i].classList.remove("displayHidden");
            }
        } else if (children[i].classList.contains("displayHidden") == false) {
            children[i].classList.add("displayHidden");
        }
    }
    return false;
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

//API
function CheckCollectionName(form, resultID) {
    document.getElementById(resultID).innerHTML = "Add to Collection"
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
    xhttp.open("GET", "/api/Collection?CollectionName="+form.elements["CollectionName"].value, true);
    xhttp.send();
    return false;
}