package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	_ "net/http/pprof"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo"
	"github.com/labstack/gommon/log"
)

const Limit = 20
const NazotteLimit = 50

var db *sqlx.DB
var db2 *sqlx.DB
var mySQLConnectionData *MySQLConnectionEnv
var mySQLConnectionData2 *MySQLConnectionEnv
var chairSearchCondition ChairSearchCondition
var estateSearchCondition EstateSearchCondition

var chairResponseMap = NewChairResponseMap()
var estateResponseMap = NewEstateResponseMap()
var chairListCache = new(ChairListCache)
var estateListCache = new(EstateListCache)

var estateMap = NewEstateMap()

type ChairResponseMap struct {
	M  map[string]*[]byte
	Mu sync.RWMutex
}

func (crm *ChairResponseMap) Add(key string, c *[]byte) {
	crm.Mu.Lock()
	defer crm.Mu.Unlock()
	crm.M[key] = c
}

func (icm *ChairResponseMap) Get(key string) *[]byte {
	icm.Mu.RLock()
	defer icm.Mu.RUnlock()
	return icm.M[key]
}

func NewChairResponseMap() *ChairResponseMap {
	m := map[string]*[]byte{}
	return &ChairResponseMap{M: m}
}

type EstateResponseMap struct {
	M  map[string]*[]byte
	Mu sync.RWMutex
}

func (erm *EstateResponseMap) Add(key string, e *[]byte) {
	erm.Mu.Lock()
	defer erm.Mu.Unlock()
	erm.M[key] = e
}

func (erm *EstateResponseMap) Get(key string) *[]byte {
	erm.Mu.RLock()
	defer erm.Mu.RUnlock()
	return erm.M[key]
}

func NewEstateResponseMap() *EstateResponseMap {
	m := map[string]*[]byte{}
	return &EstateResponseMap{M: m}
}

type ChairListCache struct {
	Res *[]byte
	Mu  sync.RWMutex
}

func (cc *ChairListCache) Update(newValue *[]byte) {
	cc.Mu.Lock()
	defer cc.Mu.Unlock()
	cc.Res = newValue
}

func (cc *ChairListCache) Get() *[]byte {
	cc.Mu.RLock()
	defer cc.Mu.RUnlock()
	return cc.Res
}

type EstateListCache struct {
	Res *[]byte
	Mu  sync.RWMutex
}

func (ec *EstateListCache) Update(newValue *[]byte) {
	ec.Mu.Lock()
	defer ec.Mu.Unlock()
	ec.Res = newValue
}

func (ec *EstateListCache) Get() *[]byte {
	ec.Mu.RLock()
	defer ec.Mu.RUnlock()
	return ec.Res
}

type EstateMap struct {
	m  map[int64]*Estate
	mu sync.RWMutex
}

func (em *EstateMap) Add(e *Estate) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.m[e.ID] = e
}

func (em *EstateMap) Get(id int64) *Estate {
	em.mu.RLock()
	defer em.mu.RUnlock()
	return em.m[id]
}

func NewEstateMap() *EstateMap {
	m := map[int64]*Estate{}
	return &EstateMap{m: m}
}

type InitializeResponse struct {
	Language string `json:"language"`
}

type Chair struct {
	ID          int64  `db:"id" json:"id"`
	Name        string `db:"name" json:"name"`
	Description string `db:"description" json:"description"`
	Thumbnail   string `db:"thumbnail" json:"thumbnail"`
	Price       int64  `db:"price" json:"price"`
	Height      int64  `db:"height" json:"height"`
	Width       int64  `db:"width" json:"width"`
	Depth       int64  `db:"depth" json:"depth"`
	Color       string `db:"color" json:"color"`
	Features    string `db:"features" json:"features"`
	Kind        string `db:"kind" json:"kind"`
	Popularity  int64  `db:"popularity" json:"-"`
	Stock       int64  `db:"stock" json:"-"`
}

type ChairSearchResponse struct {
	Count  int64   `json:"count"`
	Chairs []Chair `json:"chairs"`
}

type ChairListResponse struct {
	Chairs []Chair `json:"chairs"`
}

//Estate ??????
type Estate struct {
	ID          int64   `db:"id" json:"id"`
	Thumbnail   string  `db:"thumbnail" json:"thumbnail"`
	Name        string  `db:"name" json:"name"`
	Description string  `db:"description" json:"description"`
	Latitude    float64 `db:"latitude" json:"latitude"`
	Longitude   float64 `db:"longitude" json:"longitude"`
	Address     string  `db:"address" json:"address"`
	Rent        int64   `db:"rent" json:"rent"`
	DoorHeight  int64   `db:"door_height" json:"doorHeight"`
	DoorWidth   int64   `db:"door_width" json:"doorWidth"`
	Features    string  `db:"features" json:"features"`
	Popularity  int64   `db:"popularity" json:"-"`
}

//EstateSearchResponse estate/search??????????????????????????????
type EstateSearchResponse struct {
	Count   int64    `json:"count"`
	Estates []Estate `json:"estates"`
}

type EstateListResponse struct {
	Estates []Estate `json:"estates"`
}

type Coordinate struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Coordinates struct {
	Coordinates []Coordinate `json:"coordinates"`
}

type Range struct {
	ID  int64 `json:"id"`
	Min int64 `json:"min"`
	Max int64 `json:"max"`
}

type RangeCondition struct {
	Prefix string   `json:"prefix"`
	Suffix string   `json:"suffix"`
	Ranges []*Range `json:"ranges"`
}

type ListCondition struct {
	List []string `json:"list"`
}

type EstateSearchCondition struct {
	DoorWidth  RangeCondition `json:"doorWidth"`
	DoorHeight RangeCondition `json:"doorHeight"`
	Rent       RangeCondition `json:"rent"`
	Feature    ListCondition  `json:"feature"`
}

type ChairSearchCondition struct {
	Width   RangeCondition `json:"width"`
	Height  RangeCondition `json:"height"`
	Depth   RangeCondition `json:"depth"`
	Price   RangeCondition `json:"price"`
	Color   ListCondition  `json:"color"`
	Feature ListCondition  `json:"feature"`
	Kind    ListCondition  `json:"kind"`
}

type BoundingBox struct {
	// TopLeftCorner ??????????????????????????????????????????????????????????????????????????????
	TopLeftCorner Coordinate
	// BottomRightCorner ??????????????????????????????????????????????????????????????????????????????
	BottomRightCorner Coordinate
}

type MySQLConnectionEnv struct {
	Host     string
	Port     string
	User     string
	DBName   string
	Password string
}

type RecordMapper struct {
	Record []string

	offset int
	err    error
}

func (r *RecordMapper) next() (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.offset >= len(r.Record) {
		r.err = fmt.Errorf("too many read")
		return "", r.err
	}
	s := r.Record[r.offset]
	r.offset++
	return s, nil
}

func (r *RecordMapper) NextInt() int {
	s, err := r.next()
	if err != nil {
		return 0
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		r.err = err
		return 0
	}
	return i
}

func (r *RecordMapper) NextFloat() float64 {
	s, err := r.next()
	if err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		r.err = err
		return 0
	}
	return f
}

func (r *RecordMapper) NextString() string {
	s, err := r.next()
	if err != nil {
		return ""
	}
	return s
}

func (r *RecordMapper) Err() error {
	return r.err
}

func NewMySQLConnectionEnv() *MySQLConnectionEnv {
	return &MySQLConnectionEnv{
		Host:     getEnv("MYSQL_HOST", "192.168.33.11"),
		Port:     getEnv("MYSQL_PORT", "3306"),
		User:     getEnv("MYSQL_USER", "isucon"),
		DBName:   getEnv("MYSQL_DBNAME", "isuumo"),
		Password: getEnv("MYSQL_PASS", "isucon"),
	}
}

func NewMySQLConnectionEnv2() *MySQLConnectionEnv {
	return &MySQLConnectionEnv{
		Host:     getEnv("MYSQL_HOST2", "192.168.33.12"),
		Port:     getEnv("MYSQL_PORT", "3306"),
		User:     getEnv("MYSQL_USER", "isucon"),
		DBName:   getEnv("MYSQL_DBNAME", "isuumo"),
		Password: getEnv("MYSQL_PASS", "isucon"),
	}
}

func getEnv(key, defaultValue string) string {
	val := os.Getenv(key)
	if val != "" {
		return val
	}
	return defaultValue
}

//ConnectDB isuumo?????????????????????????????????
func (mc *MySQLConnectionEnv) ConnectDB() (*sqlx.DB, error) {
	dsn := fmt.Sprintf("%v:%v@tcp(%v:%v)/%v?interpolateParams=true", mc.User, mc.Password, mc.Host, mc.Port, mc.DBName)
	return sqlx.Open("mysql", dsn)
}

func init() {
	jsonText, err := ioutil.ReadFile("../fixture/chair_condition.json")
	if err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
	json.Unmarshal(jsonText, &chairSearchCondition)

	jsonText, err = ioutil.ReadFile("../fixture/estate_condition.json")
	if err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
	json.Unmarshal(jsonText, &estateSearchCondition)
}

func main() {

	go func() {
		fmt.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	// Echo instance
	e := echo.New()
	e.Debug = false
	e.Logger.SetLevel(log.OFF)

	// Middleware
	// e.Use(middleware.Recover())

	// Initialize
	e.POST("/initialize", initialize)

	// Chair Handler
	e.GET("/api/chair/:id", getChairDetail)
	e.POST("/api/chair", postChair)
	e.GET("/api/chair/search", searchChairs)
	e.GET("/api/chair/low_priced", getLowPricedChair)
	e.GET("/api/chair/search/condition", getChairSearchCondition)
	e.POST("/api/chair/buy/:id", buyChair)

	// Estate Handler
	e.GET("/api/estate/:id", getEstateDetail)
	e.POST("/api/estate", postEstate)
	e.GET("/api/estate/search", searchEstates)
	e.GET("/api/estate/low_priced", getLowPricedEstate)
	e.POST("/api/estate/req_doc/:id", postEstateRequestDocument)
	e.POST("/api/estate/nazotte", searchEstateNazotte)
	e.GET("/api/estate/search/condition", getEstateSearchCondition)
	e.GET("/api/recommended_estate/:id", searchRecommendedEstateWithChair)

	mySQLConnectionData = NewMySQLConnectionEnv()
	mySQLConnectionData2 = NewMySQLConnectionEnv2()

	var err error
	db, err = mySQLConnectionData.ConnectDB()
	if err != nil {
		e.Logger.Fatalf("DB connection failed : %v", err)
	}
	db.SetMaxOpenConns(10)
	defer db.Close()

	db2, err = mySQLConnectionData2.ConnectDB()
	if err != nil {
		e.Logger.Fatalf("DB connection failed : %v", err)
	}
	db2.SetMaxOpenConns(10)
	defer db2.Close()

	// use unix domain socket
	socket_file := "/var/run/app.sock"
	os.Remove(socket_file)

	l, err := net.Listen("unix", socket_file)
	if err != nil {
		e.Logger.Fatal(err)
	}

	err = os.Chmod(socket_file, 0777)
	if err != nil {
		e.Logger.Fatal(err)
	}

	e.Listener = l
	e.Logger.Fatal(e.Start(""))

	// // Start server
	// serverPort := fmt.Sprintf(":%v", getEnv("SERVER_PORT", "5000"))
	// e.Logger.Fatal(e.Start(serverPort))
}

func initialize(c echo.Context) error {
	sqlDir := filepath.Join("..", "mysql", "db")
	paths := []string{
		filepath.Join(sqlDir, "0_Schema.sql"),
		filepath.Join(sqlDir, "2_DummyChairData.sql"),
	}

	for _, p := range paths {
		sqlFile, _ := filepath.Abs(p)
		cmdStr := fmt.Sprintf("mysql -h %v -u %v -p%v -P %v %v < %v",
			mySQLConnectionData.Host,
			mySQLConnectionData.User,
			mySQLConnectionData.Password,
			mySQLConnectionData.Port,
			mySQLConnectionData.DBName,
			sqlFile,
		)
		if err := exec.Command("bash", "-c", cmdStr).Run(); err != nil {
			c.Logger().Errorf("Initialize script error : %v", err)
			return c.NoContent(http.StatusInternalServerError)
		}
	}

	paths2 := []string{
		filepath.Join(sqlDir, "0_Schema.sql"),
		filepath.Join(sqlDir, "1_DummyEstateData.sql"),
	}

	for _, p := range paths2 {
		sqlFile, _ := filepath.Abs(p)
		cmdStr := fmt.Sprintf("mysql -h %v -u %v -p%v -P %v %v < %v",
			mySQLConnectionData2.Host,
			mySQLConnectionData.User,
			mySQLConnectionData.Password,
			mySQLConnectionData.Port,
			mySQLConnectionData.DBName,
			sqlFile,
		)
		if err := exec.Command("bash", "-c", cmdStr).Run(); err != nil {
			c.Logger().Errorf("Initialize script error : %v", err)
			return c.NoContent(http.StatusInternalServerError)
		}
	}

	chairResponseMap.Mu.Lock()
	defer chairResponseMap.Mu.Unlock()
	chairResponseMap.M = map[string]*[]byte{}

	estateResponseMap.Mu.Lock()
	defer estateResponseMap.Mu.Unlock()
	estateResponseMap.M = map[string]*[]byte{}

	chairListCache.Update(nil)
	estateListCache.Update(nil)

	estateMap = NewEstateMap()
	estates := []Estate{}
	db2.Select(&estates, "SELECT id,name,description,thumbnail,address,latitude,longitude,rent,door_height,door_width,features,popularity FROM estates")
	for _, e := range estates {
		estateMap.Add(&e)
	}

	return c.JSON(http.StatusOK, InitializeResponse{
		Language: "go",
	})
}

func getChairDetail(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.Echo().Logger.Errorf("Request parameter \"id\" parse error : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	chair := Chair{}
	query := `SELECT id,name,description,thumbnail,price,height,width,depth,color,features,kind,popularity,stock FROM chair WHERE id = ?`
	err = db.Get(&chair, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.Echo().Logger.Infof("requested id's chair not found : %v", id)
			return c.NoContent(http.StatusNotFound)
		}
		c.Echo().Logger.Errorf("Failed to get the chair from id : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	} else if chair.Stock <= 0 {
		c.Echo().Logger.Infof("requested id's chair is sold out : %v", id)
		return c.NoContent(http.StatusNotFound)
	}

	return c.JSON(http.StatusOK, chair)
}

func postChair(c echo.Context) error {
	header, err := c.FormFile("chairs")
	if err != nil {
		fmt.Printf("failed to get form file: %v", err)
		return c.NoContent(http.StatusBadRequest)
	}
	f, err := header.Open()
	if err != nil {
		fmt.Printf("failed to open form file: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		fmt.Printf("failed to read csv: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	tx, err := db.Begin()
	if err != nil {
		fmt.Printf("failed to begin tx: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	defer tx.Rollback()

	placeholders := make([]string, len(records))
	params := make([]interface{}, len(records)*13)

	for i, row := range records {
		rm := RecordMapper{Record: row}
		id := rm.NextInt()
		name := rm.NextString()
		description := rm.NextString()
		thumbnail := rm.NextString()
		price := rm.NextInt()
		height := rm.NextInt()
		width := rm.NextInt()
		depth := rm.NextInt()
		color := rm.NextString()
		features := rm.NextString()
		kind := rm.NextString()
		popularity := rm.NextInt()
		stock := rm.NextInt()
		if err := rm.Err(); err != nil {
			fmt.Printf("failed to read record: %v", err)
			return c.NoContent(http.StatusBadRequest)
		}
		placeholders[i] = "(?,?,?,?,?,?,?,?,?,?,?,?,?)"
		params[13*i] = id
		params[13*i+1] = name
		params[13*i+2] = description
		params[13*i+3] = thumbnail
		params[13*i+4] = price
		params[13*i+5] = height
		params[13*i+6] = width
		params[13*i+7] = depth
		params[13*i+8] = color
		params[13*i+9] = features
		params[13*i+10] = kind
		params[13*i+11] = popularity
		params[13*i+12] = stock
	}

	_, err = tx.Exec("INSERT INTO chair(id, name, description, thumbnail, price, height, width, depth, color, features, kind, popularity, stock) VALUES"+strings.Join(placeholders, ","), params...)
	if err != nil {
		fmt.Printf("failed to insert chair: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	chairResponseMap.Mu.Lock()
	chairListCache.Mu.Lock()

	if err := tx.Commit(); err != nil {
		fmt.Printf("failed to commit tx: %v", err)
		chairResponseMap.Mu.Unlock()
		chairListCache.Mu.Unlock()
		return c.NoContent(http.StatusInternalServerError)
	}
	chairResponseMap.M = map[string]*[]byte{}
	chairResponseMap.Mu.Unlock()
	chairListCache.Res = nil
	chairListCache.Mu.Unlock()

	return c.NoContent(http.StatusCreated)
}

func searchChairs(c echo.Context) error {

	cached := chairResponseMap.Get(c.QueryString())
	if cached != nil {
		return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, *cached)
	}

	conditions := make([]string, 0)
	params := make([]interface{}, 0)

	if c.QueryParam("priceRangeId") != "" {
		chairPrice, err := getRange(chairSearchCondition.Price, c.QueryParam("priceRangeId"))
		if err != nil {
			c.Echo().Logger.Infof("priceRangeID invalid, %v : %v", c.QueryParam("priceRangeId"), err)
			return c.NoContent(http.StatusBadRequest)
		}

		if chairPrice.Min != -1 {
			conditions = append(conditions, "price >= ?")
			params = append(params, chairPrice.Min)
		}
		if chairPrice.Max != -1 {
			conditions = append(conditions, "price < ?")
			params = append(params, chairPrice.Max)
		}
	}

	if c.QueryParam("heightRangeId") != "" {
		chairHeight, err := getRange(chairSearchCondition.Height, c.QueryParam("heightRangeId"))
		if err != nil {
			c.Echo().Logger.Infof("heightRangeIf invalid, %v : %v", c.QueryParam("heightRangeId"), err)
			return c.NoContent(http.StatusBadRequest)
		}

		if chairHeight.Min != -1 {
			conditions = append(conditions, "height >= ?")
			params = append(params, chairHeight.Min)
		}
		if chairHeight.Max != -1 {
			conditions = append(conditions, "height < ?")
			params = append(params, chairHeight.Max)
		}
	}

	if c.QueryParam("widthRangeId") != "" {
		chairWidth, err := getRange(chairSearchCondition.Width, c.QueryParam("widthRangeId"))
		if err != nil {
			c.Echo().Logger.Infof("widthRangeID invalid, %v : %v", c.QueryParam("widthRangeId"), err)
			return c.NoContent(http.StatusBadRequest)
		}

		if chairWidth.Min != -1 {
			conditions = append(conditions, "width >= ?")
			params = append(params, chairWidth.Min)
		}
		if chairWidth.Max != -1 {
			conditions = append(conditions, "width < ?")
			params = append(params, chairWidth.Max)
		}
	}

	if c.QueryParam("depthRangeId") != "" {
		chairDepth, err := getRange(chairSearchCondition.Depth, c.QueryParam("depthRangeId"))
		if err != nil {
			c.Echo().Logger.Infof("depthRangeId invalid, %v : %v", c.QueryParam("depthRangeId"), err)
			return c.NoContent(http.StatusBadRequest)
		}

		if chairDepth.Min != -1 {
			conditions = append(conditions, "depth >= ?")
			params = append(params, chairDepth.Min)
		}
		if chairDepth.Max != -1 {
			conditions = append(conditions, "depth < ?")
			params = append(params, chairDepth.Max)
		}
	}

	if c.QueryParam("kind") != "" {
		conditions = append(conditions, "kind = ?")
		params = append(params, c.QueryParam("kind"))
	}

	if c.QueryParam("color") != "" {
		conditions = append(conditions, "color = ?")
		params = append(params, c.QueryParam("color"))
	}

	if c.QueryParam("features") != "" {
		for _, f := range strings.Split(c.QueryParam("features"), ",") {
			conditions = append(conditions, "features LIKE CONCAT('%', ?, '%')")
			params = append(params, f)
		}
	}

	if len(conditions) == 0 {
		c.Echo().Logger.Infof("Search condition not found")
		return c.NoContent(http.StatusBadRequest)
	}

	conditions = append(conditions, "stock > 0")

	page, err := strconv.Atoi(c.QueryParam("page"))
	if err != nil {
		c.Logger().Infof("Invalid format page parameter : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	perPage, err := strconv.Atoi(c.QueryParam("perPage"))
	if err != nil {
		c.Logger().Infof("Invalid format perPage parameter : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	searchQuery := "SELECT id,name,description,thumbnail,price,height,width,depth,color,features,kind,popularity,stock FROM chair WHERE "
	countQuery := "SELECT COUNT(*) FROM chair WHERE "
	searchCondition := strings.Join(conditions, " AND ")
	limitOffset := " ORDER BY minus_popularity ASC, id ASC LIMIT ? OFFSET ?"

	var res ChairSearchResponse
	err = db.Get(&res.Count, countQuery+searchCondition, params...)
	if err != nil {
		c.Logger().Errorf("searchChairs DB execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	chairs := []Chair{}
	params = append(params, perPage, page*perPage)
	err = db.Select(&chairs, searchQuery+searchCondition+limitOffset, params...)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, ChairSearchResponse{Count: 0, Chairs: []Chair{}})
		}
		c.Logger().Errorf("searchChairs DB execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	res.Chairs = chairs

	resJson, _ := json.Marshal(res)
	chairResponseMap.Add(c.QueryString(), &resJson)

	return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, resJson)
}

func buyChair(c echo.Context) error {
	m := echo.Map{}
	if err := c.Bind(&m); err != nil {
		c.Echo().Logger.Infof("post buy chair failed : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	_, ok := m["email"].(string)
	if !ok {
		c.Echo().Logger.Info("post buy chair failed : email not found in request body")
		return c.NoContent(http.StatusBadRequest)
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.Echo().Logger.Infof("post buy chair failed : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	tx, err := db.Beginx()
	if err != nil {
		c.Echo().Logger.Errorf("failed to create transaction : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	defer tx.Rollback()

	var chair Chair
	err = tx.QueryRowx("SELECT id,name,description,thumbnail,price,height,width,depth,color,features,kind,popularity,stock FROM chair WHERE id = ? AND stock > 0 FOR UPDATE", id).StructScan(&chair)
	if err != nil {
		if err == sql.ErrNoRows {
			c.Echo().Logger.Infof("buyChair chair id \"%v\" not found", id)
			return c.NoContent(http.StatusNotFound)
		}
		c.Echo().Logger.Errorf("DB Execution Error: on getting a chair by id : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	_, err = tx.Exec("UPDATE chair SET stock = stock - 1 WHERE id = ?", id)
	if err != nil {
		c.Echo().Logger.Errorf("chair stock update failed : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	if chair.Stock == 1 {
		chairResponseMap.Mu.Lock()
		chairListCache.Mu.Lock()
	}

	err = tx.Commit()
	if err != nil {
		c.Echo().Logger.Errorf("transaction commit error : %v", err)
		if chair.Stock == 1 {
			chairResponseMap.Mu.Unlock()
			chairListCache.Mu.Unlock()
		}
		return c.NoContent(http.StatusInternalServerError)
	}

	if chair.Stock == 1 {
		chairResponseMap.M = map[string]*[]byte{}
		chairResponseMap.Mu.Unlock()
		chairListCache.Res = nil
		chairListCache.Mu.Unlock()
	}

	return c.NoContent(http.StatusOK)
}

func getChairSearchCondition(c echo.Context) error {
	return c.JSON(http.StatusOK, chairSearchCondition)
}

func getLowPricedChair(c echo.Context) error {
	cached := chairListCache.Get()
	if cached != nil {
		return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, *cached)
	}

	var chairs []Chair
	query := `SELECT id,name,description,thumbnail,price,height,width,depth,color,features,kind,popularity,stock FROM chair WHERE stock > 0 ORDER BY price ASC, id ASC LIMIT ?`
	err := db.Select(&chairs, query, Limit)
	if err != nil {
		if err == sql.ErrNoRows {
			c.Logger().Error("getLowPricedChair not found")
			return c.JSON(http.StatusOK, ChairListResponse{[]Chair{}})
		}
		c.Logger().Errorf("getLowPricedChair DB execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	res := ChairListResponse{Chairs: chairs}

	resJson, _ := json.Marshal(res)
	chairListCache.Update(&resJson)

	return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, resJson)
}

func getEstateDetail(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.Echo().Logger.Infof("Request parameter \"id\" parse error : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	cached := estateMap.Get(int64(id))
	if cached != nil {
		return c.JSON(http.StatusOK, *cached)
	}

	var estate Estate
	err = db2.Get(&estate, "SELECT id,name,description,thumbnail,address,latitude,longitude,rent,door_height,door_width,features,popularity FROM estate WHERE id = ?", id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.Echo().Logger.Infof("getEstateDetail estate id %v not found", id)
			return c.NoContent(http.StatusNotFound)
		}
		c.Echo().Logger.Errorf("Database Execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	estateMap.Add(&estate)

	return c.JSON(http.StatusOK, estate)
}

func getRange(cond RangeCondition, rangeID string) (*Range, error) {
	RangeIndex, err := strconv.Atoi(rangeID)
	if err != nil {
		return nil, err
	}

	if RangeIndex < 0 || len(cond.Ranges) <= RangeIndex {
		return nil, fmt.Errorf("Unexpected Range ID")
	}

	return cond.Ranges[RangeIndex], nil
}

func postEstate(c echo.Context) error {
	header, err := c.FormFile("estates")
	if err != nil {
		c.Logger().Errorf("failed to get form file: %v", err)
		return c.NoContent(http.StatusBadRequest)
	}
	f, err := header.Open()
	if err != nil {
		c.Logger().Errorf("failed to open form file: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		c.Logger().Errorf("failed to read csv: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	tx, err := db2.Begin()
	if err != nil {
		c.Logger().Errorf("failed to begin tx: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	defer tx.Rollback()

	placeholders := make([]string, len(records))
	params := make([]interface{}, len(records)*12)
	for i, row := range records {
		rm := RecordMapper{Record: row}
		id := rm.NextInt()
		name := rm.NextString()
		description := rm.NextString()
		thumbnail := rm.NextString()
		address := rm.NextString()
		latitude := rm.NextFloat()
		longitude := rm.NextFloat()
		rent := rm.NextInt()
		doorHeight := rm.NextInt()
		doorWidth := rm.NextInt()
		features := rm.NextString()
		popularity := rm.NextInt()
		if err := rm.Err(); err != nil {
			c.Logger().Errorf("failed to read record: %v", err)
			return c.NoContent(http.StatusBadRequest)
		}
		placeholders[i] = "(?,?,?,?,?,?,?,?,?,?,?,?)"
		params[12*i] = id
		params[12*i+1] = name
		params[12*i+2] = description
		params[12*i+3] = thumbnail
		params[12*i+4] = address
		params[12*i+5] = latitude
		params[12*i+6] = longitude
		params[12*i+7] = rent
		params[12*i+8] = doorHeight
		params[12*i+9] = doorWidth
		params[12*i+10] = features
		params[12*i+11] = popularity
	}
	_, err = tx.Exec("INSERT INTO estate(id, name, description, thumbnail, address, latitude, longitude, rent, door_height, door_width, features, popularity) VALUES"+strings.Join(placeholders, ","), params...)
	if err != nil {
		c.Logger().Errorf("failed to insert estate: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	estateResponseMap.Mu.Lock()
	estateListCache.Mu.Lock()

	if err := tx.Commit(); err != nil {
		c.Logger().Errorf("failed to commit tx: %v", err)
		estateResponseMap.Mu.Unlock()
		estateListCache.Mu.Unlock()
		return c.NoContent(http.StatusInternalServerError)
	}

	estateResponseMap.M = map[string]*[]byte{}
	estateResponseMap.Mu.Unlock()
	estateListCache.Res = nil
	estateListCache.Mu.Unlock()

	return c.NoContent(http.StatusCreated)
}

func searchEstates(c echo.Context) error {
	cached := estateResponseMap.Get(c.QueryString())
	if cached != nil {
		return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, *cached)
	}

	conditions := make([]string, 0)
	params := make([]interface{}, 0)

	if c.QueryParam("doorHeightRangeId") != "" {
		doorHeight, err := getRange(estateSearchCondition.DoorHeight, c.QueryParam("doorHeightRangeId"))
		if err != nil {
			c.Echo().Logger.Infof("doorHeightRangeID invalid, %v : %v", c.QueryParam("doorHeightRangeId"), err)
			return c.NoContent(http.StatusBadRequest)
		}

		if doorHeight.Min != -1 {
			conditions = append(conditions, "door_height >= ?")
			params = append(params, doorHeight.Min)
		}
		if doorHeight.Max != -1 {
			conditions = append(conditions, "door_height < ?")
			params = append(params, doorHeight.Max)
		}
	}

	if c.QueryParam("doorWidthRangeId") != "" {
		doorWidth, err := getRange(estateSearchCondition.DoorWidth, c.QueryParam("doorWidthRangeId"))
		if err != nil {
			c.Echo().Logger.Infof("doorWidthRangeID invalid, %v : %v", c.QueryParam("doorWidthRangeId"), err)
			return c.NoContent(http.StatusBadRequest)
		}

		if doorWidth.Min != -1 {
			conditions = append(conditions, "door_width >= ?")
			params = append(params, doorWidth.Min)
		}
		if doorWidth.Max != -1 {
			conditions = append(conditions, "door_width < ?")
			params = append(params, doorWidth.Max)
		}
	}

	if c.QueryParam("rentRangeId") != "" {
		estateRent, err := getRange(estateSearchCondition.Rent, c.QueryParam("rentRangeId"))
		if err != nil {
			c.Echo().Logger.Infof("rentRangeID invalid, %v : %v", c.QueryParam("rentRangeId"), err)
			return c.NoContent(http.StatusBadRequest)
		}

		if estateRent.Min != -1 {
			conditions = append(conditions, "rent >= ?")
			params = append(params, estateRent.Min)
		}
		if estateRent.Max != -1 {
			conditions = append(conditions, "rent < ?")
			params = append(params, estateRent.Max)
		}
	}

	if c.QueryParam("features") != "" {
		for _, f := range strings.Split(c.QueryParam("features"), ",") {
			conditions = append(conditions, "features like concat('%', ?, '%')")
			params = append(params, f)
		}
	}

	if len(conditions) == 0 {
		c.Echo().Logger.Infof("searchEstates search condition not found")
		return c.NoContent(http.StatusBadRequest)
	}

	page, err := strconv.Atoi(c.QueryParam("page"))
	if err != nil {
		c.Logger().Infof("Invalid format page parameter : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	perPage, err := strconv.Atoi(c.QueryParam("perPage"))
	if err != nil {
		c.Logger().Infof("Invalid format perPage parameter : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	searchQuery := "SELECT id,name,description,thumbnail,address,latitude,longitude,rent,door_height,door_width,features,popularity FROM estate WHERE "
	countQuery := "SELECT COUNT(*) FROM estate WHERE "
	searchCondition := strings.Join(conditions, " AND ")
	limitOffset := " ORDER BY minus_popularity ASC, id ASC LIMIT ? OFFSET ?"

	var res EstateSearchResponse
	err = db2.Get(&res.Count, countQuery+searchCondition, params...)
	if err != nil {
		c.Logger().Errorf("searchEstates DB execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	estates := []Estate{}
	params = append(params, perPage, page*perPage)
	err = db2.Select(&estates, searchQuery+searchCondition+limitOffset, params...)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, EstateSearchResponse{Count: 0, Estates: []Estate{}})
		}
		c.Logger().Errorf("searchEstates DB execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	res.Estates = estates

	resJson, _ := json.Marshal(res)
	estateResponseMap.Add(c.QueryString(), &resJson)
	return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, resJson)
}

func getLowPricedEstate(c echo.Context) error {
	cached := estateListCache.Get()
	if cached != nil {
		return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, *cached)
	}

	estates := make([]Estate, 0, Limit)
	query := `SELECT id,name,description,thumbnail,address,latitude,longitude,rent,door_height,door_width,features,popularity FROM estate ORDER BY rent ASC, id ASC LIMIT ?`
	err := db2.Select(&estates, query, Limit)
	if err != nil {
		if err == sql.ErrNoRows {
			c.Logger().Error("getLowPricedEstate not found")
			return c.JSON(http.StatusOK, EstateListResponse{[]Estate{}})
		}
		c.Logger().Errorf("getLowPricedEstate DB execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	res := EstateListResponse{Estates: estates}

	resJson, _ := json.Marshal(res)
	estateListCache.Update(&resJson)

	return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, resJson)
}

func searchRecommendedEstateWithChair(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.Logger().Infof("Invalid format searchRecommendedEstateWithChair id : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	chair := Chair{}
	query := `SELECT id,name,description,thumbnail,price,height,width,depth,color,features,kind,popularity,stock FROM chair WHERE id = ?`
	err = db.Get(&chair, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.Logger().Infof("Requested chair id \"%v\" not found", id)
			return c.NoContent(http.StatusBadRequest)
		}
		c.Logger().Errorf("Database execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	var estates []Estate
	w := chair.Width
	h := chair.Height
	d := chair.Depth
	query = `SELECT id,name,description,thumbnail,address,latitude,longitude,rent,door_height,door_width,features,popularity FROM estate WHERE (door_width >= ? AND door_height >= ?) OR (door_width >= ? AND door_height >= ?) OR (door_width >= ? AND door_height >= ?) OR (door_width >= ? AND door_height >= ?) OR (door_width >= ? AND door_height >= ?) OR (door_width >= ? AND door_height >= ?) ORDER BY minus_popularity ASC, id ASC LIMIT ?`
	err = db2.Select(&estates, query, w, h, w, d, h, w, h, d, d, w, d, h, Limit)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, EstateListResponse{[]Estate{}})
		}
		c.Logger().Errorf("Database execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.JSON(http.StatusOK, EstateListResponse{Estates: estates})
}

func searchEstateNazotte(c echo.Context) error {
	coordinates := Coordinates{}
	err := c.Bind(&coordinates)
	if err != nil {
		c.Echo().Logger.Infof("post search estate nazotte failed : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	if len(coordinates.Coordinates) == 0 {
		return c.NoContent(http.StatusBadRequest)
	}

	b := coordinates.getBoundingBox()
	estatesInPolygon := []Estate{}
	query := fmt.Sprintf(`SELECT id,name,description,thumbnail,address,latitude,longitude,rent,door_height,door_width,features,popularity FROM estate WHERE latitude <= ? AND latitude >= ? AND longitude <= ? AND longitude >= ? AND ST_Contains(ST_PolygonFromText(%s),g) ORDER BY minus_popularity ASC, id ASC LIMIT ?`, coordinates.coordinatesToText())
	err = db2.Select(&estatesInPolygon, query, b.BottomRightCorner.Latitude, b.TopLeftCorner.Latitude, b.BottomRightCorner.Longitude, b.TopLeftCorner.Longitude, NazotteLimit)
	if err == sql.ErrNoRows {
		c.Echo().Logger.Infof("select id,name,description,thumbnail,address,latitude,longitude,rent,door_height,door_width,features,popularity FROM estate where latitude ...", err)
		return c.JSON(http.StatusOK, EstateSearchResponse{Count: 0, Estates: []Estate{}})
	} else if err != nil {
		c.Echo().Logger.Errorf("database execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	var re EstateSearchResponse
	re.Estates = estatesInPolygon
	re.Count = int64(len(re.Estates))

	return c.JSON(http.StatusOK, re)
}

func postEstateRequestDocument(c echo.Context) error {
	m := echo.Map{}
	if err := c.Bind(&m); err != nil {
		c.Echo().Logger.Infof("post request document failed : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	_, ok := m["email"].(string)
	if !ok {
		c.Echo().Logger.Info("post request document failed : email not found in request body")
		return c.NoContent(http.StatusBadRequest)
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.Echo().Logger.Infof("post request document failed : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	cached := estateMap.Get(int64(id))
	if cached != nil {
		return c.NoContent(http.StatusOK)
	}

	estate := Estate{}
	query := `SELECT id,name,description,thumbnail,address,latitude,longitude,rent,door_height,door_width,features,popularity FROM estate WHERE id = ?`
	err = db2.Get(&estate, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.NoContent(http.StatusNotFound)
		}
		c.Logger().Errorf("postEstateRequestDocument DB execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	estateMap.Add(&estate)

	return c.NoContent(http.StatusOK)
}

func getEstateSearchCondition(c echo.Context) error {
	return c.JSON(http.StatusOK, estateSearchCondition)
}

func (cs Coordinates) getBoundingBox() BoundingBox {
	coordinates := cs.Coordinates
	boundingBox := BoundingBox{
		TopLeftCorner: Coordinate{
			Latitude: coordinates[0].Latitude, Longitude: coordinates[0].Longitude,
		},
		BottomRightCorner: Coordinate{
			Latitude: coordinates[0].Latitude, Longitude: coordinates[0].Longitude,
		},
	}
	for _, coordinate := range coordinates {
		if boundingBox.TopLeftCorner.Latitude > coordinate.Latitude {
			boundingBox.TopLeftCorner.Latitude = coordinate.Latitude
		}
		if boundingBox.TopLeftCorner.Longitude > coordinate.Longitude {
			boundingBox.TopLeftCorner.Longitude = coordinate.Longitude
		}

		if boundingBox.BottomRightCorner.Latitude < coordinate.Latitude {
			boundingBox.BottomRightCorner.Latitude = coordinate.Latitude
		}
		if boundingBox.BottomRightCorner.Longitude < coordinate.Longitude {
			boundingBox.BottomRightCorner.Longitude = coordinate.Longitude
		}
	}
	return boundingBox
}

func (cs Coordinates) coordinatesToText() string {
	points := make([]string, 0, len(cs.Coordinates))
	for _, c := range cs.Coordinates {
		points = append(points, fmt.Sprintf("%f %f", c.Latitude, c.Longitude))
	}
	return fmt.Sprintf("'POLYGON((%s))'", strings.Join(points, ","))
}
