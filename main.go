package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"github.com/bwmarrin/discordgo"
	"github.com/robfig/cron/v3"
	"google.golang.org/api/iterator"
)

var (
	GuildID    = flag.String("g", "", "Guild ID")
	BotToken   = flag.String("t", "", "Bot token")
	GCPProject = flag.String("p", "", "GCP Project")
)

var session *discordgo.Session
var ctx context.Context
var client *firestore.Client

const prettyDateFormat = "January 2, 2006"

type month struct {
	StartTime time.Time `json:"start_time"`
	Days      []day     `json:"days"`
}

type day struct {
	Day    int    `json:"day"`
	Prompt string `json:"prompt"`
}

func init() { flag.Parse() }

func init() {
	var err error
	session, err = discordgo.New("Bot " + *BotToken)
	if err != nil {
		log.Fatalf("Missing bot parameters: %v", err)
	}

	ctx = context.Background()
	conf := &firebase.Config{ProjectID: *GCPProject}
	app, err := firebase.NewApp(ctx, conf)
	if err != nil {
		log.Printf("Couldn't connect to Firestore, so many commands will not work: %v", err)
		return
	}

	client, err = app.Firestore(ctx)
	if err != nil {
		log.Printf("Couldn't connect to Firestore, so many commands will not work: %v", err)
		return
	}
}

var (
	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "birdass",
			Description: "Just birdass",
		},
		{
			Name:        "addrole",
			Description: "Add a role to yourself, eg pronouns or colours",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "The role to add",
					Required:    true,
				},
			},
		},
		{
			Name:        "removerole",
			Description: "Remove a role from yourself",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "rolerole",
					Description: "The role to add",
					Required:    true,
				},
			},
		},
		{
			Name:        "bigemoji",
			Description: "Emoji, but T H I C C",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "emoji",
					Description: "The emoji to biggify",
					Required:    true,
				},
			},
		},
		{
			Name:        "bogart",
			Description: "bogart",
		},
		{
			Name:        "reminder",
			Description: "Set a reminder for yourself",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "reminder",
					Description: "Thing to remind you of",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "when",
					Description: "When should I remind you? (format: 5d3h30m)",
					Required:    true,
				},
			},
		},
		{
			Name:        "suggestion",
			Description: "Make a feature request for this bot of bird and ass",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "suggestion",
					Description: "What do you want to see implemented?",
					Required:    true,
				},
			},
		},
		{
			Name:        "musicsetup",
			Description: "Sets up a music month - only works for mfcrocker",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "file",
					Description: "A URL to a text file",
					Required:    true,
				},
			},
		},
		{
			Name:        "musicmonth",
			Description: "Get the current music month, if any",
		},
		{
			Name:        "musicprompt",
			Description: "Get the prompt for a music month",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "day",
					Description: "The day to retrieve (gets today if not provided)",
					Required:    false,
				},
			},
		},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"birdass": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionApplicationCommandResponseData{
					Content: "just birdass",
				},
			})
		},
		"addrole": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionApplicationCommandResponseData{
					Flags: 64,
				},
			})
			s.GuildMemberRoleAdd(i.GuildID, i.Member.User.ID, i.Data.Options[0].RoleValue(nil, "").ID)
		},
		"removerole": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionApplicationCommandResponseData{
					Flags: 64,
				},
			})
			s.GuildMemberRoleRemove(i.GuildID, i.Member.User.ID, i.Data.Options[0].RoleValue(nil, "").ID)
		},
		"bigemoji": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			valid, _ := regexp.MatchString(`<a?:\w+:\d+>`, i.Data.Options[0].StringValue())
			if !valid {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionApplicationCommandResponseData{
						Flags: 64,
					},
				})
				return
			}
			emojiID := strings.TrimSuffix(strings.Split(i.Data.Options[0].StringValue(), ":")[2], ">")
			animated, _ := regexp.MatchString(`<a:\w+:\d+>`, i.Data.Options[0].StringValue())
			suffix := ".png?v=1"
			if animated {
				suffix = ".gif?v=1"
			}
			emojiURI := "https://cdn.discordapp.com/emojis/" + emojiID + suffix
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionApplicationCommandResponseData{
					Content: emojiURI,
				},
			})
		},
		"bogart": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionApplicationCommandResponseData{
					Content: "https://cdn.discordapp.com/emojis/721104351220727859.png?v=1",
				},
			})
		},
		"reminder": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			if client == nil {
				// We're not connected to GCP, don't let them do this
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionApplicationCommandResponseData{
						Content: "I haven't been set up to allow reminders, please moan at whoever set me up",
					},
				})
				return
			}
			timeString := i.Data.Options[1].StringValue()
			offset := 0
			parseString := timeString
			if strings.Contains(timeString, "d") {
				parseString = strings.Split(timeString, "d")[0]
				days, err := strconv.Atoi(parseString)
				if err != nil {
					s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionApplicationCommandResponseData{
							Content: "That's not the right date or time format. Example: 5d3h30m for a reminder in 5 1/2 hours",
						},
					})
					return
				}
				offset += days * 24
				parseString = strings.Split(timeString, "d")[1]
			}

			parsedDuration, err := time.ParseDuration(parseString)
			parsedOffset, _ := time.ParseDuration(strconv.Itoa(offset) + "h")
			parsedDuration += parsedOffset
			if err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionApplicationCommandResponseData{
						Content: "That's not the right date or time format. Example: 5d3h30m for a reminder in 5 1/2 hours",
					},
				})
				return
			}
			reminderTimestamp := time.Now().Add(parsedDuration)

			_, _, err = client.Collection("reminders").Add(ctx, map[string]interface{}{
				"userID":   i.Member.User.ID,
				"reminder": i.Data.Options[0].StringValue(),
				"date":     reminderTimestamp,
			})

			if err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionApplicationCommandResponseData{
						Content: "Something went wrong at my end so I didn't save your reminder",
					},
				})
				log.Printf("Error saving record to Firestore: %v", err)
				return
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionApplicationCommandResponseData{
					Content: "Okay, I've set a reminder up to remind you of " + i.Data.Options[0].StringValue(),
				},
			})
		},
		"suggestion": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionApplicationCommandResponseData{
					Flags: 64,
				},
			})
			channel, err := session.UserChannelCreate(fmt.Sprintf("%v", "147856569730596864"))
			if err != nil {
				fmt.Printf("Couldn't talk to user: %v", err)
			}
			_, err = session.ChannelMessageSend(channel.ID, "You've had a suggestion from "+i.Member.User.Username+": "+i.Data.Options[0].StringValue())
		},
		"musicsetup": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			if client == nil {
				// We're not connected to GCP, don't let them do this
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionApplicationCommandResponseData{
						Content: "I haven't been set up to allow music months, please moan at whoever set me up",
					},
				})
				return
			}
			if i.Member.User.ID != "147856569730596864" {
				// You ain't me
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionApplicationCommandResponseData{
						Content: "Please ask mfcrocker to set this up!",
					},
				})
				return
			}
			if !strings.HasSuffix(i.Data.Options[0].StringValue(), ".json") {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionApplicationCommandResponseData{
						Content: "Give me a .json file",
					},
				})
				return
			}

			resp, err := http.Get(i.Data.Options[0].StringValue())
			if err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionApplicationCommandResponseData{
						Content: "Couldn't get the file from the URL provided",
					},
				})
				return
			}
			defer resp.Body.Close()

			monthData, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionApplicationCommandResponseData{
						Content: "Error reading the file bytes",
					},
				})
				return
			}

			var musicMonth month
			err = json.Unmarshal(monthData, &musicMonth)
			if err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionApplicationCommandResponseData{
						Content: "Invalid JSON",
					},
				})
				return
			}

			_, _, err = client.Collection("musicmonth").Add(ctx, musicMonth)

			if err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionApplicationCommandResponseData{
						Content: "Something went wrong at my end so I didn't save the month",
					},
				})
				log.Printf("Error saving record to Firestore: %v", err)
				return
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionApplicationCommandResponseData{
					Content: "Okay, I've set up a music month beginning on " + musicMonth.StartTime.Format(prettyDateFormat),
				},
			})
		},
		"musicmonth": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			if client == nil {
				// We're not connected to GCP, don't let them do this
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionApplicationCommandResponseData{
						Content: "I haven't been set up to allow music months, please moan at whoever set me up",
					},
				})
				return
			}
			now := time.Now()
			// Give a couple of days grace on this - would normally be -now.Day() + 1
			currentMonthStart := now.AddDate(0, 0, -now.Day()-1)
			currentMonthEnd := now.AddDate(0, 1, -now.Day())
			iter := client.Collection("musicmonth").Where("StartTime", ">", currentMonthStart).OrderBy("StartTime", firestore.Asc).Limit(1).Documents(ctx)
			docs, _ := iter.GetAll()
			if len(docs) == 0 {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionApplicationCommandResponseData{
						Content: "No music month planned",
					},
				})
				return
			}

			var currentMonth month
			docs[0].DataTo(&currentMonth)
			var response strings.Builder

			if currentMonth.StartTime.After(currentMonthEnd) {
				response.WriteString("There's no current music month; the next begins on " + currentMonth.StartTime.Format(prettyDateFormat) + "\n")
			} else {
				response.WriteString("Current music month: \n")
			}
			response.WriteString("```")
			for _, day := range currentMonth.Days {
				response.WriteString(currentMonth.StartTime.Format("January") + " " + strconv.Itoa(day.Day) + ": " + day.Prompt + "\n")
			}
			response.WriteString("```")

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionApplicationCommandResponseData{
					Content: response.String(),
				},
			})
		},
		"musicprompt": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			now := time.Now()
			day := now.Day()
			if len(i.Data.Options) > 0 {
				day = int(i.Data.Options[0].IntValue())
			}

			// Give a couple of days grace on this - would normally be -now.Day() + 1
			currentMonthStart := now.AddDate(0, 0, -now.Day()-1)
			currentMonthEnd := now.AddDate(0, 1, -now.Day())
			iter := client.Collection("musicmonth").Where("StartTime", ">", currentMonthStart).Where("StartTime", "<", currentMonthEnd).OrderBy("StartTime", firestore.Asc).Limit(1).Documents(ctx)
			docs, _ := iter.GetAll()
			if len(docs) == 0 {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionApplicationCommandResponseData{
						Content: "No currently active music month",
					},
				})
				return
			}
			var currentMonth month
			docs[0].DataTo(&currentMonth)
			for _, prompt := range currentMonth.Days {
				if prompt.Day == day {
					s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionApplicationCommandResponseData{
							Content: "Prompt for day " + strconv.Itoa(prompt.Day) + ": " + prompt.Prompt,
						},
					})
					return
				}
			}
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionApplicationCommandResponseData{
					Content: "No prompt found for day " + strconv.Itoa(day),
				},
			})
		},
	}
)

func init() {
	session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.Data.Name]; ok {
			h(s, i)
		}
	})
}

func checkReminders() {
	iter := client.Collection("reminders").Where("date", "<", time.Now()).Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			fmt.Printf("Something went wrong getting reminders on a cron: %v", err)
			break
		}
		channel, err := session.UserChannelCreate(fmt.Sprintf("%v", doc.Data()["userID"]))
		if err != nil {
			fmt.Printf("Couldn't talk to user: %v", err)
		}
		_, err = session.ChannelMessageSend(channel.ID, "Hi there! You asked me to remind you about "+fmt.Sprintf("%v", doc.Data()["reminder"])+" - this is that reminder!")
		if err != nil {
			fmt.Printf("Error trying to remind someone: %v", err)
		}

		doc.Ref.Delete(ctx)
	}
}

func main() {
	var c *cron.Cron
	if client != nil {
		c := cron.New()
		c.AddFunc("@every 1m", func() { checkReminders() })
		c.Start()
		defer client.Close()
	}
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Println("Ready to birdass")
	})
	err := session.Open()
	if err != nil {
		log.Fatalf("Couldn't connect to Discord: %v", err)
	}

	for _, v := range commands {
		_, err := session.ApplicationCommandCreate(session.State.User.ID, *GuildID, v)
		if err != nil {
			log.Fatalf("Couldn't create '%v' command: %v", v.Name, err)
		}
	}

	defer session.Close()

	stop := make(chan os.Signal)
	signal.Notify(stop, os.Interrupt)
	<-stop
	log.Println("Shutting down bird asses")
	if c != nil {
		c.Stop()
	}
}
