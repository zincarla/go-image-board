//This class intended to provide some auto-complete capabilities for tag boxes.
class AutoCompleteBox {
    //TextBox is the box we are autocompleting for, SuggestionBox is a div to show results in.
	constructor(TextBox, SuggestionBox) {
		var self=this;
		this.completionResults = [];
		this.textBox = TextBox;
		this.suggestionBox = SuggestionBox;
		this.lastSetSuggestion =-1;
		this.textBox.addEventListener('keydown',function (e) {self.keyHandler(e);},false);
        this.textBox.addEventListener('keyup',function (e) {self.keyHandlerUp(e);},false);
        this.textBox.autocomplete="off";
	}

    //Gets the last word being typed, and provides suggestions for it
	getSuggestion() {
		var self=this;
		var seedWord = this.textBox.value.split(" ");
		seedWord = seedWord[seedWord.length-1];
		if (seedWord.length > 0) {
			//console.log("Seed word is:"+seedWord);
            this.completionResults = [];
            this.lastSetSuggestion = -1;

			//Do search for completions
            this.SearchTags(seedWord);
		} else {
			this.completionResults = [];
			this.suggestionBox.innerHTML = "";
		}
    }

    //Fills the suggestion div based on the completionResults array
    FillSuggestionList() {
        var self = this;
        var ulNode = document.createElement("ul");
        ulNode.classList.add("suggestionList");
        for (var I = 0; I < this.completionResults.length; I++) {
            var liNode = document.createElement("li");
            var valNode = document.createElement("input");
            valNode.type="hidden";
            valNode.value=I;
            liNode.innerHTML = this.completionResults[I];
            liNode.appendChild(valNode);
            liNode.addEventListener('click',function (e) {self.setSuggestion(this.getElementsByTagName("input")[0].value); self.ClearSuggestionList();},false);
            ulNode.appendChild(liNode);
        }

        if (this.suggestionBox.childNodes.length > 0) {
            this.suggestionBox.replaceChild(ulNode,this.suggestionBox.firstChild);
        } else {
            this.suggestionBox.appendChild(ulNode);
        }
    }

    //Clears the suggestion list and div.
    ClearSuggestionList() {
        self.completionResults = [];
        self.lastSetSuggestion = -1;
        this.suggestionBox.innerHTML="";
    }
    
    //Actually connects to server REST API and pulls tag names down
    SearchTags(seedWord) {
        var self = this;
        var xhttp = new XMLHttpRequest();
        xhttp.onreadystatechange = function() {
            if (this.readyState == 4 && this.status == 200 ) {
                //Get JSON object, and ID
                var Result = JSON.parse(xhttp.responseText);
                //Out with the old
                self.completionResults = [];
                self.lastSetSuggestion = -1;
                //In with new
                if (Result.Tags) {
                    var ulNode = document.createElement("ul");
                    ulNode.classList.add("suggestionList");
                    for (var I = 0; I<Result.Tags.length; I++) {
                        self.completionResults.push(Result.Tags[I].Name)
                    }
                    self.FillSuggestionList();
                } else {
                    self.ClearSuggestionList();
                }
            } else if (this.readyState == 4) {
                console.log("Error occurred: "+this.status+" - "+this.statusText+". From server: "+xhttp.responseText)
            }
        };
        xhttp.open("GET", "/api/TagName?tagNameQuery="+seedWord, true);
        xhttp.send();
        return false;
    }

    //Selects a suggestion from the completionResults array
	setSuggestion(index) {
		if (index < 0 || index >= this.completionResults.length) {
			return;
		}
		if (this.completionResults.length > 0) {
			if (this.suggestionBox.childNodes.length > 0) {
				for (var i=0; i < this.suggestionBox.childNodes[0].childNodes.length; i++) {
					if (this.suggestionBox.childNodes[0].childNodes[i].classList.contains("highlightedAutoComplete")) {
						this.suggestionBox.childNodes[0].childNodes[i].classList.remove("highlightedAutoComplete");
					}
				}
			}
			this.lastSetSuggestion=index;
			if (this.suggestionBox.childNodes.length > 0 && this.suggestionBox.childNodes[0].childNodes.length > index) {
				this.suggestionBox.childNodes[0].childNodes[index].classList.add("highlightedAutoComplete");
			}
			var seedWord = this.textBox.value.split(" ");
			seedWord = seedWord[seedWord.length-1];
			this.textBox.value = this.textBox.value.substring(0,this.textBox.value.length-seedWord.length)+this.completionResults[index];
		}
	}

    //Calls new suggestions when the tabkey is pressed
	keyHandler(e) {
		var TABKEY = 9;
		if(e.keyCode == TABKEY) {
			var newSuggestion = this.lastSetSuggestion+1;
			if (newSuggestion >= this.completionResults.length || newSuggestion < 0) {
				newSuggestion =0;
			}
			this.setSuggestion(newSuggestion);
			if(e.preventDefault) {
				e.preventDefault();
			}
			return false;
		}
	}

    //Reloads autocomplete options when anything other than tab is pressed
	keyHandlerUp(e) {
		var TABKEY = 9;
		if(e.keyCode != TABKEY) {
			this.getSuggestion();
		}
	}
}