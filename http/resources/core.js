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