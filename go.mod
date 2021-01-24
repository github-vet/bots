module github.com/github-vet/bots

go 1.15

require (
	cloud.google.com/go v0.65.0
	github.com/BurntSushi/toml v0.3.1
	github.com/Masterminds/squirrel v1.5.0
	github.com/PuerkitoBio/goquery v1.6.1
	github.com/StefanSchroeder/odtfontfind v0.0.0-20191225171436-0375aee1203e
	github.com/alecthomas/participle v0.7.1
	github.com/ankyra/escape-core v0.0.0-20181128112917-39783202d4ad
	github.com/aws/aws-sdk-go v1.36.31
	github.com/axgle/mahonia v0.0.0-20180208002826-3358181d7394
	github.com/babydb/babydb v0.0.0-20190528070427-5bb49d27866e
	github.com/beewit/beekit v0.0.0-20180416030343-0747d370e464
	github.com/beewit/wechat-ai v0.0.0-20180130054210-ba51ba57433e
	github.com/bjeanes/go-edn v0.0.0-20150829124132-aa12bdc743c2
	github.com/boltdb/bolt v1.3.1
	github.com/boombuler/barcode v1.0.1 // indirect
	github.com/brimstone/dwork v0.0.0-20170407013452-3ed78a1644ac
	github.com/cbroglie/mustache v1.2.0
	github.com/cea-hpc/sshproxy v1.3.3
	github.com/chrislusf/glow v0.0.0-20181102060906-4c40a2717eee
	github.com/christoph-k/go-http-jwtfilter v0.0.0-20170118203555-5d368de2d396
	github.com/chromedp/chromedp v0.6.5
	github.com/colinmarc/hdfs v1.1.3
	github.com/coreos/etcd v3.3.25+incompatible
	github.com/dakraid/skyrimSaveMaster v0.0.0-20190530162313-d6fe2e7e52da
	github.com/davecgh/go-spew v1.1.1
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/dmlittle/scenery v0.1.5
	github.com/docker/docker v20.10.2+incompatible
	github.com/fatih/color v1.9.0
	github.com/fluent/fluent-bit-go v0.0.0-20201210173045-3fd1e0486df2
	github.com/garyburd/redigo v1.6.2
	github.com/ghodss/yaml v1.0.0
	github.com/gin-contrib/cors v1.3.1
	github.com/gin-gonic/gin v1.6.3
	github.com/glenn-brown/golang-pkg-pcre v0.0.0-20120522223659-48bb82a8b8ce
	github.com/go-ini/ini v1.62.0
	github.com/go-sql-driver/mysql v1.5.0
	github.com/go-xorm/xorm v0.7.9
	github.com/gocql/gocql v0.0.0-20201215165327-e49edf966d90
	github.com/godoctor/godoctor v0.0.0-20200702010311-8433dcb3dc61
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.4.3
	github.com/google/btree v1.0.0
	github.com/google/go-cmp v0.5.2
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-github/v32 v32.1.0
	github.com/google/uuid v1.2.0 // indirect
	github.com/gopherjs/gopherjs v0.0.0-20200217142428-fce0ec30dd00
	github.com/gopherjs/websocket v0.0.0-20191103002815-9a42957e2b3a // indirect
	github.com/gorilla/mux v1.7.3
	github.com/gorilla/schema v1.2.0
	github.com/gorilla/websocket v1.4.2
	github.com/hillu/go-xxtea v0.0.0-20160426195705-316144b06424
	github.com/jalilbengoufa/pixicoreAPI v0.0.0-20181227020558-ed6e75ed102f
	github.com/jinzhu/gorm v1.9.16
	github.com/jmoiron/sqlx v1.2.1-0.20190826204134-d7d95172beb5
	github.com/jonbodner/proteus v0.14.0
	github.com/kelseyhightower/confd v0.16.0
	github.com/kr/pretty v0.2.1 // indirect
	github.com/kr/pty v1.1.8
	github.com/labstack/echo v3.3.10+incompatible // indirect
	github.com/labstack/gommon v0.3.0 // indirect
	github.com/leomfelicissimo/akiva v0.0.0-20190421015837-af45ddbf0414
	github.com/lib/pq v1.8.0
	github.com/lxn/win v0.0.0-20201111105847-2a20daff6a55
	github.com/matishsiao/goInfo v0.0.0-20200404012835-b5f882ee2288
	github.com/mattn/anko v0.1.8
	github.com/mattn/go-sqlite3 v2.0.3+incompatible
	github.com/micro/go-micro v1.18.0
	github.com/micro/go-plugins/broker/nsq v0.0.0-20200119172437-4fe21aa238fd
	github.com/micro/go-plugins/registry/etcdv3 v0.0.0-20200119172437-4fe21aa238fd
	github.com/micro/go-web v1.0.0
	github.com/moby/term v0.0.0-20201216013528-df9cb8a40635 // indirect
	github.com/olekukonko/tablewriter v0.0.4
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.9.0
	github.com/op/go-logging v0.0.0-20160315200505-970db520ece7
	github.com/osrg/gobgp v2.0.0+incompatible
	github.com/packethost/packngo v0.5.1
	github.com/pkg/errors v0.9.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/psilva261/timsort v1.0.0
	github.com/reedobrien/s3cp v0.0.0-20180326162604-597b9e47af18
	github.com/rs/xid v1.2.1
	github.com/rwcarlsen/goexif v0.0.0-20190401172101-9e8deecbddbd
	github.com/sanandak/seg v0.0.0-20190615215440-0c86c9e9b8a9
	github.com/sclevine/agouti v3.0.0+incompatible
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/cobra v1.0.0
	github.com/spf13/viper v1.7.0
	github.com/streadway/amqp v1.0.0
	github.com/stretchr/testify v1.6.1
	github.com/tecbot/gorocksdb v0.0.0-20191217155057-f0fad39f321c
	github.com/thoas/go-funk v0.7.0
	github.com/vishvananda/netlink v1.1.0
	github.com/willf/bitset v1.1.11-0.20200630133818-d5bec3311243 // indirect
	go.etcd.io/etcd v3.3.25+incompatible
	go.uber.org/zap v1.16.0 // indirect
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad
	golang.org/x/mod v0.4.0 // indirect
	golang.org/x/net v0.0.0-20201202161906-c7110b5ffcbb
	golang.org/x/oauth2 v0.0.0-20201109201403-9fd604954f58
	golang.org/x/tools v0.0.0-20210108195828-e2f9c7f1fc8e
	gonum.org/v1/gonum v0.8.2
	google.golang.org/grpc v1.31.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22
	gopkg.in/xmlpath.v2 v2.0.0-20150820204837-860cbeca3ebc
	gopkg.in/yaml.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776 // indirect
	honnef.co/go/js/dom v0.0.0-20200509013220-d4405f7ab4d8
	vimagination.zapto.org/form v0.0.0-20180612134117-f2bf592c0031
	vimagination.zapto.org/gedcom v0.0.0-20200222183852-4296e4597194
	vimagination.zapto.org/gopherjs v0.0.0-20180612134603-5a689ece0d3e
	vimagination.zapto.org/httpbuffer v0.0.0-20181223165411-0065008eba70
	vimagination.zapto.org/httpencoding v0.0.0-20200308201051-11896c622dd3 // indirect
	vimagination.zapto.org/httpgzip v0.0.0-20200314191905-465154311b64
	vimagination.zapto.org/httplog v0.0.0-20200314192016-1bf691c78252
	vimagination.zapto.org/httprpc v0.0.0-20200308200257-769901961fa5
	vimagination.zapto.org/httpwrap v0.0.0-20200308200802-e7fec2f9434b // indirect
	vimagination.zapto.org/memio v0.0.0-20200222190306-588ebc67b97d // indirect
	vimagination.zapto.org/pagination v0.0.0-20180612140750-1a62c67dcb4a
	vimagination.zapto.org/parser v0.0.0-20190322094028-915635b05e24 // indirect
	vimagination.zapto.org/webserver v0.0.0-20200320192106-da4d284c422c
)
