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
                newElem.innerHTML =  "<td>"+Result.Users[I].Name+"</td><td>"+Result.Users[I].ID+"</td><td>"+Result.Users[I].Permissions+"</td><td>"+Result.Users[I].Disabled+"</td><td>"+Result.Users[I].CreationTime+"</td>"
                table.appendChild(newElem);
            }
            GenerateUserSearchMenu(form.elements["userName"].value, Result.ServerStride, pageStart, Result.ResultCount, 'userResultPageMenu', formID)
        } else if (this.readyState == 4) {
            document.getElementById("userResultCount").innerText = "Error occurred: "+this.status+" - "+this.statusText+". From server: "+xhttp.responseText
        }
    };
    xhttp.open("GET", "/api/User?userNameQuery="+form.elements["userName"].value+"&PageStart="+pageStart, true);
    xhttp.send();
    return false;
}

function GenerateUserSearchMenu(searchQuery, pageStride, pageOffset, maxCount, targetID, formID) {
    pageMenu = "<a href=\"#\" onclick=\"return SearchUsers('"+formID+"', 0);\">&#x3C;&#x3C;</a>"
    //Short circuit on param issues
    if (pageOffset < 0) {
        pageOffset = 0
    }
    if (pageStride < 0 || maxCount <=0) {
        document.getElementById(targetID).innerHTML = "1";
        return false;
    }

    lastPage = Math.ceil(maxCount / pageStride);
    maxPage = lastPage
    currentPage = Math.floor(pageOffset/pageStride) + 1
    minPage = currentPage -3
    if (minPage < 1) {
		minPage = 1
	}
	if (maxPage > currentPage+3) {
		maxPage = currentPage + 3
    }
    
    for (processPage = minPage; processPage <= maxPage; processPage++) {
        if (processPage != currentPage) {
            pageMenu = pageMenu + ", <a href=\"#\" onclick=\"return SearchUsers('"+formID+"', "+((processPage-1)*pageStride)+");\">"+processPage+"</a>"
        } else {
            pageMenu = pageMenu + ", "+processPage
        }
    }

    pageMenu = pageMenu + ", <a href=\"#\" onclick=\"return SearchUsers('"+formID+"', "+((lastPage-1)*pageStride)+");\">&#x3E;&#x3E;</a>"

    document.getElementById(targetID).innerHTML = pageMenu;
}