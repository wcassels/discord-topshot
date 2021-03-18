package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/akamensky/argparse"
	"github.com/bwmarrin/discordgo"
	_ "github.com/go-sql-driver/mysql"
	"github.com/lus/dgc"
	"github.com/onflow/flow-go-sdk/client"
	"google.golang.org/grpc"
)

type alertGeneral struct {
	channelID         string
	playerCondition   bool
	playerName        string
	minPriceCondition bool
	minPrice          uint64
	maxPriceCondition bool
	maxPrice          uint64
}

type cardAlert struct {
	isPMAlert          bool
	discordChannelID   string
	discordUserID      string
	nameCondition      bool
	nameSubString      string
	minPriceCondition  bool
	minPrice           float64
	maxPriceCondition  bool
	maxPrice           float64
	minSerialCondition bool
	minSerial          uint32
	maxSerialCondition bool
	maxSerial          uint32
	setCondition       bool
	setSubString       string
	seriesCondition    bool
	seriesNum          uint32
}

func handleFatalErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func handleErr(err error) {
	if err != nil {
		fmt.Println(err)
	}
}

func main() {
	byteToken, err := ioutil.ReadFile("token.txt")
	handleFatalErr(err)

	dg, err := discordgo.New("Bot " + string(byteToken))
	handleFatalErr(err)

	// Start listening
	err = dg.Open()
	handleFatalErr(err)

	router := dgc.Create(&dgc.Router{
		Prefixes: []string{"!"},
	})

	// Cryptoslam command
	router.RegisterCmd(&dgc.Command{
		Name:        "slam",
		Aliases:     []string{"cryptoslam"},
		Description: "Links to a TS account's Cryptoslam page",
		Usage:       "slam <user>",
		Example:     "slam Pranked",
		IgnoreCase:  true,
		Handler:     slamHandler,
	})

	// Flow Address command
	router.RegisterCmd(&dgc.Command{
		Name:        "flow",
		Description: "Returns a TS account's flow address",
		Usage:       "address <user>",
		Example:     "address Pranked",
		IgnoreCase:  true,
		Handler: func(ctx *dgc.Ctx) {
			addressHandler(ctx, "Flow")
		},
	})

	router.RegisterCmd(&dgc.Command{
		Name:        "dapper",
		Description: "Returns a TS account's Dapper ID",
		Usage:       "dapper <user>",
		Example:     "dapper Pranked",
		IgnoreCase:  true,
		Handler: func(ctx *dgc.Ctx) {
			addressHandler(ctx, "Dapper")
		},
	})

	var alerts []cardAlert

	// Alerts command
	router.RegisterCmd(&dgc.Command{
		Name:        "alert",
		Description: "Add a new alert",
		Handler: func(ctx *dgc.Ctx) {
			alertHandler(ctx, &alerts)
		},
	})

	// Add help command
	router.RegisterDefaultHelpCommand(dg, nil)

	router.Initialize(dg)

	// Get flowClient
	flowClient, err := client.New("access.mainnet.nodes.onflow.org:9000", grpc.WithInsecure())
	if err != nil {
		fmt.Println("Error accessing mainnet node: ", err)
		return
	}
	// test connection
	err = flowClient.Ping(context.Background())
	if err != nil {
		fmt.Println("Error pinging node: ", err)
	}

	// db, _ := sql.Open("mysql", "root:mV&6eLntD^yKYnqYzWtZ@tcp(127.0.0.1:3306)/testdb")
	// err = db.Ping()
	// handleErr(err)
	//
	// defer db.Close()
	//
	// fmt.Println("Successfully conected to DB")
	// testChannelID := "646832566665478148"

	salesChan := make(chan *SaleMoment, 100)

	go sales(dg, flowClient, salesChan)

	// CHANGE THIS TO YOUR DISCORD ID
	discordID := ""

	go sendAlerts(dg, salesChan, &alerts, discordID)

	fmt.Println("Bot running. Press Ctrl-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Stop
	fmt.Println("Shutting down...")
	dg.Close()

}

func slamHandler(ctx *dgc.Ctx) {
	args := ctx.Arguments
	address, err := GetAddress(args.Get(0).Raw(), "Flow")
	if err != nil {
		// s.ChannelMessageSend(m.ChannelID, fmt.Sprint(err))
		// panic(err)
	}
	err = ctx.RespondText(fmt.Sprintf("https://cryptoslam.io/owner/0x%s", address))
	if err != nil {
		panic(err)
	}
}

func addressHandler(ctx *dgc.Ctx, which string) {
	args := ctx.Arguments
	address, err := GetAddress(args.Get(0).Raw(), which)
	if err != nil {
		fmt.Println(err)
	}
	_ = ctx.RespondText(address)
}

func alertHandler(ctx *dgc.Ctx, alerts *[]cardAlert) {
	parser := argparse.NewParser("alert", "No-one is reading this anyway")
	name := parser.String("n", "name", &argparse.Options{
		Required: false,
		Default:  nil,
	})
	minSerial := parser.Int("t", "minserial", &argparse.Options{
		Required: false,
		Default:  -1,
	})
	maxSerial := parser.Int("u", "maxserial", &argparse.Options{
		Required: false,
		Default:  -1,
	})
	minPrice := parser.Float("p", "minprice", &argparse.Options{
		Required: false,
		Default:  -1.0,
	})
	maxPrice := parser.Float("q", "maxprice", &argparse.Options{
		Required: false,
		Default:  -1.0,
	})
	set := parser.String("s", "set", &argparse.Options{
		Required: false,
		Default:  nil,
	})
	series := parser.Int("r", "series", &argparse.Options{
		Required: false,
		Default:  -1,
	})
	err := parser.Parse(strings.Split(strings.TrimPrefix(ctx.Event.Content, "!alert"), " "))
	if err != nil {
		panic(err)
	}
	newAlert := cardAlert{
		isPMAlert:          true,
		discordChannelID:   "",
		discordUserID:      ctx.Event.Author.ID,
		nameCondition:      (*name != ""),
		nameSubString:      *name,
		minPriceCondition:  (*minPrice != -1),
		minPrice:           *minPrice,
		maxPriceCondition:  (*maxPrice != -1),
		maxPrice:           *maxPrice,
		minSerialCondition: (*minSerial != -1),
		minSerial:          uint32(*minSerial),
		maxSerialCondition: (*maxSerial != -1),
		maxSerial:          uint32(*maxSerial),
		setCondition:       (*set != ""),
		setSubString:       *set,
		seriesCondition:    (*series != -1),
		seriesNum:          uint32(*series),
	}

	*alerts = append(*alerts, newAlert)
	ctx.RespondText(newAlert.summary())
}

func (alert cardAlert) summary() string {
	var sb strings.Builder
	sb.WriteString("```OK, I'll ")
	if alert.isPMAlert {
		sb.WriteString("PM you ")
	} else {
		sb.WriteString("post here ")
	}
	sb.WriteString("when a moment is purchased and the following conditions are met:\n")

	if alert.nameCondition {
		sb.WriteString(fmt.Sprintf("Name contains %s\n", alert.nameSubString))
	}
	if alert.setCondition {
		sb.WriteString(fmt.Sprintf("Set name contains %s\n", alert.setSubString))
	}
	if alert.seriesCondition {
		sb.WriteString(fmt.Sprintf("Series is %d\n", alert.seriesNum))
	}
	if alert.minPriceCondition {
		sb.WriteString(fmt.Sprintf("Price is above %.0f\n", alert.minPrice))
	}
	if alert.maxPriceCondition {
		sb.WriteString(fmt.Sprintf("Price is below %.0f\n", alert.maxPrice))
	}
	if alert.minSerialCondition {
		sb.WriteString(fmt.Sprintf("Serial is above %d\n", alert.minSerial))
	}
	if alert.maxSerialCondition {
		sb.WriteString(fmt.Sprintf("Serial is below %d\n", alert.maxSerial))
	}
	sb.WriteString("```")
	return sb.String()
}

// func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
// 	// Ignore bot messages
// 	if m.Author.ID == s.State.User.ID {
// 		return
// 	}
//
// 	if strings.HasPrefix(m.Content, "!test") {
// 		ID, err := GetPlayIDFromURL(strings.Split(m.Content, " ")[1])
// 		if err != nil {
// 			s.ChannelMessageSend(m.ChannelID, fmt.Sprint(err))
// 		}
// 		s.ChannelMessageSend(m.ChannelID, ID)
// 	}
// }

func sales(ds *discordgo.Session, fc *client.Client, salesChan chan<- *SaleMoment) {
	latestBlock, err := fc.GetLatestBlock(context.Background(), false)
	handleErr(err)

	prevHeight := latestBlock.Height
	time.Sleep(5 * time.Second)

	for {
		fmt.Println("Next loop")
		latestBlock, err := fc.GetLatestBlock(context.Background(), false)
		handleErr(err)
		currHeight := latestBlock.Height

		blockEvents, _ := fc.GetEventsForHeightRange(context.Background(), client.EventRangeQuery{
			Type:        "A.c1e4f4f4c4257510.Market.MomentPurchased",
			StartHeight: prevHeight,
			EndHeight:   currHeight,
		})
		prevHeight = latestBlock.Height

		if len(blockEvents) == 0 {
			fmt.Println("No purchase events")
			time.Sleep(2 * time.Second)
		} else {
			fmt.Println("events:", len(blockEvents))
			for _, blockEvent := range blockEvents {
				fmt.Println("sub-events:", len(blockEvent.Events))
				for _, purchaseEvent := range blockEvent.Events {
					e := MomentPurchasedEvent(purchaseEvent.Value)
					saleMoment, err := GetSaleMomentFromOwnerAtBlock(fc, blockEvent.Height-1, *e.Seller(), e.Id())
					if err != nil {
						fmt.Println("Error executing script for moment ", e, blockEvent.Height-1)
						continue
					}
					// _, err = ds.ChannelMessageSend(testChannelID, saleMoment.String())
					// handleErr(err)
					salesChan <- saleMoment
				}
			}
		}

	}
}

func sendAlerts(ds *discordgo.Session, salesChan <-chan *SaleMoment, alerts *[]cardAlert, discordID string) {
	for {
		moment := <-salesChan
		for _, alert := range *alerts {
			if doesAlertApply(alert, moment) {
				channel, _ := ds.UserChannelCreate(discordID)
				_, err := ds.ChannelMessageSend(channel.ID, moment.String())
				handleErr(err)
			} else {
				fmt.Println("Does not apply")
			}
		}
	}
}

func doesAlertApply(alert cardAlert, moment *SaleMoment) bool {
	if alert.nameCondition {
		if !strings.Contains(strings.ToLower(moment.Play()["FullName"]), strings.ToLower(alert.nameSubString)) {
			return false
		}
	}

	if alert.minPriceCondition {
		if moment.Price() < alert.minPrice {
			return false
		}
	}

	if alert.maxPriceCondition {
		if moment.Price() > alert.maxPrice {
			return false
		}
	}

	if alert.minSerialCondition {
		if moment.SerialNumber() < alert.minSerial {
			return false
		}
	}

	if alert.maxSerialCondition {
		if moment.SerialNumber() > alert.maxSerial {
			return false
		}
	}

	if alert.setCondition {
		if !strings.Contains(strings.ToLower(moment.SetName()), strings.ToLower(alert.setSubString)) {
			return false
		}
	}

	if alert.seriesCondition {
		if moment.Series() != alert.seriesNum {
			return false
		}
	}

	return true
}
