package mariadbplugin

import (
	"errors"
	"go-image-board/interfaces"
	"go-image-board/logging"
	"strings"
)

//SearchCollections performs a search for collections (Returns a list of CollectionInformation a result count and an error/nil)
//If you edit this function, consider SearchImages for a similar change
func (DBConnection *MariaDBPlugin) SearchCollections(Tags []interfaces.TagInformation, PageStart uint64, PageStride uint64) ([]interfaces.CollectionInformation, uint64, error) {
	//Cleanup input for use in code below
	//Specifically we separate the include, the exclude and metatags into their own lists
	var IncludeTags []uint64
	var ExcludeTags []uint64
	var MetaTags []interfaces.TagInformation
	for _, tag := range Tags {
		if tag.Exists && tag.IsAlias == false && tag.IsMeta == false {
			if tag.Exclude {
				ExcludeTags = append(ExcludeTags, tag.ID)
			} else {
				IncludeTags = append(IncludeTags, tag.ID)
			}
		} else if tag.Exists && tag.IsMeta {
			MetaTags = append(MetaTags, tag)
		}
	}

	//Initialize output
	var ToReturn []interfaces.CollectionInformation
	var MaxResults uint64

	//Construct SQL Query

	//This is the start of the query we want
	sqlQuery := `SELECT ID, Name, IFNULL(Preview.Location,"") as Location, IFNULL(Counts.Members,0) as Members `
	sqlCountQuery := `SELECT COUNT(*) `
	if len(IncludeTags) == 0 {
		sqlQuery = sqlQuery + `FROM Collections `
		sqlCountQuery = sqlCountQuery + `FROM Collections `
	} else {
		sqlQuery = sqlQuery + `FROM (
			SELECT CollectionID as ID, Name, COUNT(*) as MatchingTags
			FROM CollectionTags 
			INNER JOIN Collections ON CollectionTags.CollectionID=Collections.ID `
		sqlCountQuery = sqlCountQuery + `FROM ( 
			SELECT CollectionID as ID, Name, COUNT(*) as MatchingTags
			FROM CollectionTags 
			INNER JOIN Collections ON CollectionTags.CollectionID=Collections.ID `
	}

	//Now for the variable piece
	sqlWhereClause := ""
	if len(IncludeTags) > 0 {
		sqlWhereClause = sqlWhereClause + "WHERE TagID IN (?" + strings.Repeat(",?", len(IncludeTags)-1) + ") "
	}
	if len(ExcludeTags) > 0 {
		if len(IncludeTags) > 0 {
			sqlWhereClause += "AND "
		} else {
			sqlWhereClause += "WHERE "
		}
		sqlWhereClause += "Collections.ID NOT IN (SELECT DISTINCT CollectionID FROM CollectionTags WHERE TagID IN (?" + strings.Repeat(",?", len(ExcludeTags)-1) + ")) "
	}

	//And add any metatags
	if len(MetaTags) > 0 {
		for _, tag := range MetaTags {
			metaTagQuery := "AND "
			if sqlWhereClause == "" {
				metaTagQuery = "WHERE "
			}
			metaTagQuery = metaTagQuery + "Collections." + tag.Name + " "
			comparator := tag.Comparator
			if tag.Exclude {
				comparator = getInvertedComparator(comparator)
			}
			if comparator == "" {
				return ToReturn, 0, errors.New("Failed to invert query to negate on " + tag.Name)
			}
			metaTagQuery = metaTagQuery + comparator + " ? "

			sqlWhereClause = sqlWhereClause + metaTagQuery
		}
	}

	//Special difference here compares to searchImages, this gets Location for a cover of the collection of sorts
	previewCountPortion := `LEFT JOIN (
		SELECT CollectionID, Count(*) as Members
		FROM CollectionMembers
		GROUP BY CollectionID
	) Counts ON Counts.CollectionID = ID
	LEFT JOIN (
		SELECT Location, CollectionMembers.CollectionID as CollectionID, MIN(CollectionMembers.OrderWeight)
		FROM Images
		INNER JOIN CollectionMembers ON CollectionMembers.ImageID = Images.ID
		GROUP BY CollectionMembers.CollectionID
	) Preview ON Preview.CollectionID = ID `

	if len(IncludeTags) > 0 {
		sqlQuery = sqlQuery + sqlWhereClause + `GROUP BY CollectionID) InnerStatement ` + previewCountPortion + `WHERE MatchingTags = ? `
		sqlCountQuery = sqlCountQuery + sqlWhereClause + `GROUP BY CollectionID) InnerStatement WHERE MatchingTags = ? `
	} else {
		sqlQuery = sqlQuery + previewCountPortion + sqlWhereClause
		sqlCountQuery = sqlCountQuery + sqlWhereClause
	}

	//Add Order
	sqlQuery = sqlQuery + `ORDER BY ID
		DESC LIMIT ? OFFSET ?;`

	//Now construct arguments list. Order must follow query order
	/*
		Inclusive Tags
		Exclusive Tags
		Inclusive Tag Count
		<However we pause here to run count query, as that one does not have limits>
		Max Amount of results to return (Stride)
		Offset (Start)
	*/
	queryArray := []interface{}{}
	//Add inclusive tags to our queryArray
	for _, tag := range IncludeTags {
		queryArray = append(queryArray, tag)
	}
	//Add the exclusive tags
	for _, tag := range ExcludeTags {
		queryArray = append(queryArray, tag)
	}
	//Add values for metatags
	for _, tag := range MetaTags {
		queryArray = append(queryArray, tag.MetaValue)
	}

	//Add inclusive tag count, but only if we have any
	if len(IncludeTags) > 0 {
		queryArray = append(queryArray, len(IncludeTags))
	}

	//Run the count query (Count query does not use start/stride, so run this before we add those)
	err := DBConnection.DBHandle.QueryRow(sqlCountQuery, queryArray...).Scan(&MaxResults)
	if err != nil {
		logging.LogInterface.WriteLog("MariaDBPlugin", "SearchCollections", "*", "ERROR", []string{"Error running search query", sqlCountQuery, err.Error()})
		return nil, 0, err
	}

	//Add rest of arguments now that we have max result count
	queryArray = append(queryArray, PageStride)
	queryArray = append(queryArray, PageStart)

	//Now we have query and args, run the query
	rows, err := DBConnection.DBHandle.Query(sqlQuery, queryArray...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	//Placeholders for data returned by each row
	var CollectionID uint64
	var Name string
	var Location string
	var Members uint64
	//For each row
	for rows.Next() {
		//Parse out the data
		err := rows.Scan(&CollectionID, &Name, &Location, &Members)
		if err != nil {
			return nil, 0, err
		}
		//Add this result to ToReturn
		ToReturn = append(ToReturn, interfaces.CollectionInformation{Name: Name, ID: CollectionID, Location: Location, Members: Members})
	}
	return ToReturn, MaxResults, nil
}
