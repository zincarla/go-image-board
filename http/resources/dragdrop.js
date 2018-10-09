var draggedElement= null;
var proposedReplace = null;
var dragClass = "dragging";
var proposeReplaceClass = "proposeReplace";
var dragOffsetX=0;
var dragOffsetY=0;

function updateDrag(e) {
	if (draggedElement !== null) {
		draggedElement.style.left = (dragOffsetX + e.clientX)+"px"
		draggedElement.style.top = (dragOffsetY + e.clientY)+"px"
	}
}

function suggestDragReplace(toSuggest) {
	if (toSuggest !== draggedElement) {
		if (draggedElement !== null) {
			if (proposedReplace !== null) {
				proposedReplace.classList.remove(proposeReplaceClass);
			}
			proposedReplace = toSuggest
			proposedReplace.classList.add(proposeReplaceClass);
		}
	}
}

function clearDragSuggestion() {
	if (proposedReplace !== null) {
		proposedReplace.classList.remove(proposeReplaceClass);
		proposedReplace = null
	}
}

function startDrag(e, toDrag) {
	if (draggedElement === null) {
		clearDragSuggestion();
		draggedElement = toDrag;
		dragOffsetX = (draggedElement.getBoundingClientRect().left + document.documentElement.scrollLeft) - e.clientX
		dragOffsetY = (draggedElement.getBoundingClientRect().top + document.documentElement.scrollTop) - e.clientY
		draggedElement.classList.add(dragClass);
		updateDrag(e);
	}
}

function stopDrag(e) {
	if (draggedElement !== null) {
		if (proposedReplace !== null) {
			draggedElement.parentElement.insertBefore(draggedElement, proposedReplace);
			clearDragSuggestion();
		}
		draggedElement.style.left = null
		draggedElement.style.top = null
		
		dragOffsetX = 0
		dragOffsetY = 0
		draggedElement.classList.remove(dragClass);
		
		draggedElement = null;
	}
}

//This is for save function on collectionreorder
function generateOrder(form) {
	toReturn = "" //to set NewOrder to

	//Grab all DIVs from central area, and parse them into comma list
	gridChildren = document.getElementById("ImageGridContainer").childNodes
	for (index = 0; index < gridChildren.length; ++index) {
		if (gridChildren[index].id != null) {
			if (gridChildren[index].id.startsWith("image-")) {
				toReturn += gridChildren[index].id.substr(6)+","
			}
		}
	}

	if (toReturn.length > 2) { //Remove trailing comma
		toReturn = toReturn.substr(0, toReturn.length -1)
	}
	//Parse them into comma separated string (toReturn)
	form.elements["NewOrder"].value = toReturn;
	return true;
}