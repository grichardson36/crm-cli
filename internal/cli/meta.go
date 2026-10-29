package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/schema"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// cmdMeta is the agent-facing description of one command. It is the single place where every
// command declares whether it changes data, how to call it, and what it prints. applyMeta copies
// it into cmd.Annotations and cmd.Example so `crm schema` and `--help` read the same text.
type cmdMeta struct {
	mutates     bool
	destructive bool     // cannot be undone with crm: needs --yes
	examples    []string // copy-pasteable command lines
	fields      string   // JSON output fields, comma separated
	target      string   // entity of the ID arguments, for the --dry-run preview
	target2     string   // entity of the second ID, for targetArgs "pair"
	targetArgs  string   // which args are IDs: "first", "all", "pair" (two IDs) or "tag" (entity type, then ID)
	noDryRun    bool     // do not add the generic --dry-run flag
}

const (
	personFields = "id,uuid,first_name,last_name,name,email,phone,title,company,location,notes,summary,birthday,keep_in_touch,last_contacted_at,is_favorite,priority_tier,emails,phones,handles,links,created_at,updated_at"
	orgFields    = "id,uuid,name,domain,industry,notes,summary,website,size,city,region,country,linkedin_url,description,logo_path,links,created_at,updated_at"
	stageFields  = "position,name,outcome,in_use,removed"
	dealFields   = "id,uuid,title,stage,value,person_ids,org_ids,org_id,notes,closed_at,links,created_at,updated_at"
	taskFields   = "id,uuid,title,priority,completed,description,person_ids,org_ids,deal_ids,due_at,recurrence,tags,completed_at,links,created_at,updated_at"
	logFields    = "id,uuid,type,subject,content,direction,occurred_at,person_ids,org_ids,deal_ids,created_at,updated_at"
	tagFields    = "id,name"
	syncFields   = "source,status,created,updated,skipped,inbox,errors,dry_run,note,run_id,since,next_cursor"
)

// commandMeta is keyed by command path below the root.
var commandMeta = map[string]cmdMeta{
	// people
	"person add": {mutates: true, noDryRun: true, fields: personFields, examples: []string{
		`crm person add "Jane Smith" --email jane@example.com --company "Acme Corp" -f json`,
		`crm person add "Bob Jones" --keep-in-touch "1 mon" --birthday 1985-03-14 --dry-run`,
		`crm person add "Sam Lee #friend every 3m !high" --dry-run`}},
	"person list": {fields: personFields, examples: []string{
		`crm person list -f json`, `crm person list --tag vip --limit 10 -f json`}},
	"person show": {fields: personFields, examples: []string{`crm person show 1 -f json`}},
	"person edit": {mutates: true, fields: personFields, target: "person", targetArgs: "first", examples: []string{
		`crm person edit 1 --title CTO --company "Acme Corp"`, `crm person edit 1 --keep-in-touch "14 days" --dry-run`}},
	"person delete": {mutates: true, target: "person", targetArgs: "all", examples: []string{
		`crm person delete 3`, `crm person delete 3 4 --dry-run`}},
	"person archive": {mutates: true, target: "person", targetArgs: "all", examples: []string{`crm person archive 3`}},
	"person restore": {mutates: true, target: "person", targetArgs: "all", examples: []string{
		`crm person restore 3`, `crm person restore 3 --dry-run`}},
	"person purge": {mutates: true, destructive: true, target: "person", targetArgs: "all", examples: []string{
		`crm person purge 3 --dry-run`, `crm person purge 3 --yes`}},
	"person merge": {mutates: true, fields: "fields,added,moved,dry_run", examples: []string{
		`crm person merge 1 3 --dry-run -f json`, `crm person merge 1 3 --prefer title=3`}},
	"person email":      {examples: []string{`crm person email add 1 jane@work.example --label Work`}},
	"person email add":  {mutates: true, fields: personFields, target: "person", targetArgs: "first", examples: []string{`crm person email add 1 jane@work.example --label Work --primary`}},
	"person email rm":   {mutates: true, fields: personFields, target: "person", targetArgs: "first", examples: []string{`crm person email rm 1 jane@work.example`}},
	"person phone":      {examples: []string{`crm person phone add 1 "+1 555 0100" --label Mobile`}},
	"person phone add":  {mutates: true, fields: personFields, target: "person", targetArgs: "first", examples: []string{`crm person phone add 1 "+1 555 0100" --label Mobile`}},
	"person phone rm":   {mutates: true, fields: personFields, target: "person", targetArgs: "first", examples: []string{`crm person phone rm 1 "+1 555 0100"`}},
	"person handle":     {examples: []string{`crm person handle set 1 linkedin https://linkedin.com/in/jane`}},
	"person handle set": {mutates: true, fields: personFields, target: "person", targetArgs: "first", examples: []string{`crm person handle set 1 linkedin https://linkedin.com/in/jane`}},
	"person handle rm":  {mutates: true, fields: personFields, target: "person", targetArgs: "first", examples: []string{`crm person handle rm 1 linkedin`}},
	"person contacted": {mutates: true, fields: personFields, target: "person", targetArgs: "first", examples: []string{
		`crm person contacted 1`, `crm person contacted 1 --at 2026-10-01`}},
	"person relate": {mutates: true, target: "person", targetArgs: "first", examples: []string{
		`crm person relate 1 2 --type colleague --notes "Met at ReactConf"`}},
	"person relationships": {target: "person", fields: "id,person_id,related_person_id,name,type,notes", examples: []string{`crm person relationships 1 -f json`}},
	"person unrelate":      {mutates: true, examples: []string{`crm person unrelate 1`}},

	// link kinds and roles
	"relation-kind":      {examples: []string{`crm relation-kind list`}},
	"relation-kind list": {fields: relationKindFields, examples: []string{`crm relation-kind list -f json`, `crm relation-kind list --type org`}},
	"relation-kind add": {mutates: true, noDryRun: true, fields: relationKindFields, examples: []string{
		`crm relation-kind add neighbor`, `crm relation-kind add godparent --inverse godchild`, `crm relation-kind add partner --type org`}},
	"relation-kind rm": {mutates: true, noDryRun: true, examples: []string{`crm relation-kind rm neighbor`}},
	"roles":            {fields: "role", examples: []string{`crm roles -f json`, `crm roles --relation deal_person`}},

	// keep in touch
	"due": {fields: "person_id,id,name,keep_in_touch,last_seen,due_at,days_overdue", examples: []string{
		`crm due -f json`, `crm due --within 7 -f json`}},
	"birthdays": {fields: "person_id,id,name,date,days_until,relative,turning", examples: []string{
		`crm birthdays -f json`, `crm birthdays --days 90 -f json`}},

	// organizations
	"purge":        {mutates: true, noDryRun: true, destructive: true, fields: "id,name,status", examples: []string{`crm purge --archived`, `crm purge --archived --type person --ids 3,4 --apply`}},
	"alias":        {examples: []string{`crm alias list`}},
	"alias list":   {fields: "alias,canonical,source,active", examples: []string{`crm alias list -f json`}},
	"alias add":    {mutates: true, noDryRun: true, examples: []string{`crm alias add zeke zachary`}},
	"alias remove": {mutates: true, noDryRun: true, examples: []string{`crm alias remove zeke`}},

	"org add":    {mutates: true, fields: orgFields, examples: []string{`crm org add "Acme Corp" --domain acme.com --industry SaaS`}},
	"org list":   {fields: orgFields, examples: []string{`crm org list -f json`}},
	"org show":   {fields: orgFields + ",people", examples: []string{`crm org show 1 --with-people -f json`}},
	"org edit":   {mutates: true, fields: orgFields, target: "organization", targetArgs: "first", examples: []string{`crm org edit 1 --domain acme.io`}},
	"org delete": {mutates: true, target: "organization", targetArgs: "first", examples: []string{`crm org delete 1`}},

	"org backfill": {mutates: true, noDryRun: true, fields: "applied,counts,groups", examples: []string{
		`crm org backfill -f json`, `crm org backfill --limit 20`, `crm org backfill --apply`}},
	"org convert":       {mutates: true, noDryRun: true, fields: "applied,counts,groups", examples: []string{`crm org convert -f json`}},
	"org ignore list":   {fields: "text,source,active", examples: []string{`crm org ignore list -f json`}},
	"org ignore add":    {mutates: true, noDryRun: true, examples: []string{`crm org ignore add "stay at home parent"`}},
	"org ignore remove": {mutates: true, noDryRun: true, examples: []string{`crm org ignore remove retired`}},
	"org dupes": {fields: "a_id,a_name,b_id,b_name,reason,merge", examples: []string{
		`crm org dupes -f json`, `crm org dupes --limit 20`, `crm org dupes --dismissed`}},
	"org dupes dismiss":   {mutates: true, noDryRun: true, examples: []string{`crm org dupes dismiss 1 2`}},
	"org dupes undismiss": {mutates: true, noDryRun: true, examples: []string{`crm org dupes undismiss 1 2`}},

	// merge log
	"merge log":  {fields: "id,kind,survivor_id,survivor_name,loser_ids,loser_names,summary,status,created_at,undone_at", examples: []string{`crm merge log -f json`, `crm merge log --kind organization --limit 5`}},
	"merge undo": {mutates: true, noDryRun: true, fields: "id,kind,survivor_id,loser_ids,status,skipped", examples: []string{`crm merge undo 3`}},
	"merge redo": {mutates: true, noDryRun: true, fields: "id,kind,survivor_id,loser_ids,status,skipped", examples: []string{`crm merge redo 3`}},

	"org merge": {mutates: true, noDryRun: true, fields: "keep_id,keep_name,drop_ids,drop_names,dry_run,fields,lost,moved", examples: []string{
		`crm org merge 1 2 3 -f json`, `crm org merge 1 2 3 --apply`}},

	// deals
	"deal add":            {mutates: true, fields: dealFields, examples: []string{`crm deal add "Website Redesign" --value 15000 --person 1 --stage proposal`}},
	"deal list":           {fields: dealFields, examples: []string{`crm deal list --open -f json`}},
	"deal show":           {fields: dealFields, examples: []string{`crm deal show 1 -f json`}},
	"deal edit":           {mutates: true, fields: dealFields, target: "deal", targetArgs: "first", examples: []string{`crm deal edit 1 --stage won --closed-at 2026-10-15`}},
	"deal delete":         {mutates: true, target: "deal", targetArgs: "first", examples: []string{`crm deal delete 1`}},
	"deal pipeline":       {fields: "stage,outcome,count,total_value", examples: []string{`crm deal pipeline -f json`}},
	"deal roles":          {examples: []string{`crm deal roles list`}},
	"deal roles list":     {fields: "position,name,in_use,removed", examples: []string{`crm deal roles list -f json`, `crm deal roles list --all`}},
	"deal roles add":      {mutates: true, noDryRun: true, fields: "position,name,in_use,removed", examples: []string{`crm deal roles add "technical evaluator"`}},
	"deal roles rename":   {mutates: true, noDryRun: true, fields: "old,new,rows", examples: []string{`crm deal roles rename "decision maker" "final decision maker"`}},
	"deal roles remove":   {mutates: true, noDryRun: true, fields: "name,moved,kept,replaced_by", examples: []string{`crm deal roles remove blocker --replace-with influencer`, `crm deal roles remove "billing contact" --keep`}},
	"deal roles reorder":  {mutates: true, noDryRun: true, fields: "position,name,in_use,removed", examples: []string{`crm deal roles reorder champion "economic buyer" signer`, `crm deal roles reorder signer --position 2`}},
	"deal roles backfill": {mutates: true, noDryRun: true, fields: "applied,counts,roles", examples: []string{`crm deal roles backfill -f json`, `crm deal roles backfill --apply`}},
	"deal stages":         {examples: []string{`crm deal stages list`}},
	"deal stages list":    {fields: stageFields, examples: []string{`crm deal stages list -f json`, `crm deal stages list --all`}},
	"deal stages add":     {mutates: true, noDryRun: true, fields: stageFields, examples: []string{`crm deal stages add qualified`}},
	"deal stages rename":  {mutates: true, noDryRun: true, fields: "old,new,rows", examples: []string{`crm deal stages rename proposal "quote sent"`}},
	"deal stages remove":  {mutates: true, noDryRun: true, fields: "name,moved,replaced_by", examples: []string{`crm deal stages remove prospect --replace-with lead`}},
	"deal stages reorder": {mutates: true, noDryRun: true, fields: stageFields, examples: []string{`crm deal stages reorder lead proposal`, `crm deal stages reorder proposal --position 2`}},
	"deal stages outcome": {mutates: true, noDryRun: true, fields: "name,old,new,closed", examples: []string{`crm deal stages outcome lost lost`, `crm deal stages outcome negotiation open`}},

	// tasks
	"task add": {mutates: true, noDryRun: true, fields: taskFields, examples: []string{
		`crm task add "Send proposal" --person 1 --due 2026-11-01 --priority high`,
		`crm task add "Call back" --person 1 --due 2026-11-01 --recurrence "7 days"`,
		`crm task add "Call back #work fri 3pm !high @Jane" --dry-run`}},
	"do": {mutates: true, noDryRun: true, fields: "input,verb,dry_run,applied,preview,subject,links,changes,warnings,problems,needs_confirm,syntax", examples: []string{
		`crm do "n Jane called about the deal #deal remind fri" --dry-run`,
		`crm do "t Jane send the contract by mon !high"`,
		`crm do "set Jane: company=Acme title=CTO" --dry-run -f json`,
		`crm do "tag note" --person 1`,
		`crm do "n coffee with @Priya" --person 1 --keep-unresolved --dry-run`}},
	"task list":  {fields: taskFields, examples: []string{`crm task list -f json`, `crm task list --overdue -f json`, `crm task list --tag work -f json`}},
	"task tag":   {mutates: true, fields: taskFields, target: "task", targetArgs: "first", examples: []string{`crm task tag 1 work urgent`}},
	"task untag": {mutates: true, fields: taskFields, target: "task", targetArgs: "first", examples: []string{`crm task untag 1 urgent`}},
	"task show":  {fields: taskFields, examples: []string{`crm task show 1 -f json`}},
	"task edit": {mutates: true, fields: taskFields, target: "task", targetArgs: "first", examples: []string{`crm task edit 1 --priority low`,
		`crm task edit 1 --due 2026-11-03 --dry-run`}},
	"task done":    {mutates: true, fields: taskFields, target: "task", targetArgs: "first", examples: []string{`crm task done 1`}},
	"task delete":  {mutates: true, target: "task", targetArgs: "first", examples: []string{`crm task delete 1`}},
	"task restore": {mutates: true, target: "task", targetArgs: "first", examples: []string{`crm task restore 1`}},

	// interactions
	"log call":    {mutates: true, fields: logFields, target: "person", targetArgs: "all", examples: []string{`crm log call 1 --subject "Intro call" --content "Agreed on Q4 timeline"`}},
	"log email":   {mutates: true, fields: logFields, target: "person", targetArgs: "all", examples: []string{`crm log email 1 --subject "Follow-up" --direction outbound`}},
	"log meeting": {mutates: true, fields: logFields, target: "person", targetArgs: "all", examples: []string{`crm log meeting 1 2 --subject "Product demo" --at "2026-10-20 14:00"`}},
	"log note":    {mutates: true, fields: logFields, target: "person", targetArgs: "all", examples: []string{`crm log note 1 --subject "Quick check-in" --content "Seemed interested"`}},
	"log message": {mutates: true, fields: logFields, target: "person", targetArgs: "all", examples: []string{`crm log message 1 --content "Sent the deck on WhatsApp"`}},
	"log add": {mutates: true, noDryRun: true, fields: logFields, target: "person", targetArgs: "first", examples: []string{
		`crm log add --type call 1 "Talked about the offsite"`,
		`crm log add --type call 1 "Talked about the offsite #trip yesterday" --dry-run`}},
	"log types": {fields: "id,uuid,name,counts_as_contact,archived,created_at,updated_at", examples: []string{
		`crm log types -f json`, `crm log types --all -f json`}},
	"log types add": {mutates: true, fields: "id,uuid,name,counts_as_contact,archived", examples: []string{`crm log types add standup --counts-as-contact=false`}},
	"log types rm":  {mutates: true, examples: []string{`crm log types rm standup`}},
	"log list":      {fields: logFields, examples: []string{`crm log list --person 1 --limit 5 -f json`}},
	"log edit":      {mutates: true, fields: logFields, target: "interaction", targetArgs: "first", examples: []string{`crm log edit 1 --content "Corrected text"`}},
	"log rm":        {mutates: true, target: "interaction", targetArgs: "first", examples: []string{`crm log rm 1`}},

	// tags
	"tag list":       {fields: tagFields, examples: []string{`crm tag list -f json`}},
	"tag apply":      {mutates: true, targetArgs: "tag", examples: []string{`crm tag apply person 1 vip`}},
	"tag remove":     {mutates: true, targetArgs: "tag", examples: []string{`crm tag remove person 1 vip`}},
	"tag show":       {fields: tagFields, examples: []string{`crm tag show person 1 -f json`}},
	"tag rename":     {mutates: true, noDryRun: true, fields: "old,name", examples: []string{`crm tag rename friends friend`}},
	"tag merge":      {mutates: true, noDryRun: true, fields: tagMergeFields, examples: []string{`crm tag merge old-tag new-tag -f json`, `crm tag merge old-tag new-tag --apply`}},
	"tag unused":     {mutates: true, noDryRun: true, fields: "dry_run,count,tags", examples: []string{`crm tag unused -f json`, `crm tag unused --apply`}},
	"tag duplicates": {mutates: true, noDryRun: true, fields: "groups,applied", examples: []string{`crm tag duplicates -f json`, `crm tag duplicates --apply`}},
	"tag tree":       {fields: "path,name,depth,virtual,own,total,children", examples: []string{`crm tag tree`, `crm tag tree -f json`}},
	"tag nest":       {mutates: true, noDryRun: true, fields: "action,dry_run,ops,renamed,merged,records,duplicates", examples: []string{`crm tag nest home hobbies --under area/life`, `crm tag nest home --under area --dry-run -f json`}},
	"tag unnest":     {mutates: true, noDryRun: true, fields: "action,dry_run,ops,renamed,merged,records,duplicates", examples: []string{`crm tag unnest area/life/home`}},
	"tag delete":     {mutates: true, destructive: true, examples: []string{`crm tag delete old-tag --dry-run`, `crm tag delete old-tag --yes`, `crm tag delete old-tag --only-this --yes`}},

	// duplicates
	"dupes": {fields: "level,ids,members,reasons", examples: []string{
		`crm dupes -f json`, `crm dupes --min high -f json`}},
	"dupes dismiss":   {mutates: true, examples: []string{`crm dupes dismiss 1 2`}},
	"dupes undismiss": {mutates: true, examples: []string{`crm dupes undismiss 1 2`}},

	// search and views
	"search":  {fields: "type,id,name,detail", examples: []string{`crm search jane -f json`, `crm search roadmap --type interaction -f json`}},
	"context": {fields: "person,organization,organizations,recent_interactions,deals,tasks,relationships,tags", examples: []string{`crm context 1 -f json`, `crm context "Jane Smith" -f json`}},
	"status":  {fields: "contacts,organizations,open_deals,deal_value,open_tasks,overdue_tasks,interactions_7days,last_sync,device_id", examples: []string{`crm status -f json`}},

	// import, sync, backup
	"import dex": {mutates: true, examples: []string{
		`crm import dex --contacts dex_contacts.csv --dry-run`, `crm import dex --contacts dex_contacts.csv --notes dex_notes.csv`}},
	"sync list":   {examples: []string{`crm sync list -f json`}},
	"sync run":    {mutates: true, fields: syncFields, examples: []string{`crm sync run SOURCE --dry-run -f json`}},
	"sync status": {examples: []string{`crm sync status -f json`}},
	"sync inbox list": {examples: []string{
		`crm sync inbox list -f json`, `crm sync inbox list --status all -f json`}},
	"sync inbox":        {examples: []string{`crm sync inbox -f json`}},
	"sync inbox accept": {mutates: true, examples: []string{`crm sync inbox accept 1`}},
	"sync inbox attach": {mutates: true, examples: []string{`crm sync inbox attach 1 2`}},
	"sync inbox ignore": {mutates: true, examples: []string{`crm sync inbox ignore 1`}},
	"backup":            {examples: []string{`crm backup ./crm-backup.db`}},

	// phone sync over CardDAV
	"dav serve": {mutates: true, noDryRun: true, examples: []string{
		`crm dav serve --password-env CRM_DAV_PASSWORD`,
		`crm dav serve --password-file ~/.crm/dav-password --user grant --base-url https://contacts.example.com --read-only`}},
	"dav info": {fields: "base_url,user,address_book,address_book_url,address_book_path,discovery_url,listen,password,steps,notes", examples: []string{
		`crm dav info -f json`, `crm dav info --base-url https://contacts.example.com --user grant`}},
	"dav status": {fields: "contacts_exposed,device_created,synced_to_device,last_request_at,last_get_at,last_put_at,conflicts,open_conflicts", examples: []string{
		`crm dav status -f json`}},
	"dav conflicts": {fields: davConflictFields, examples: []string{`crm dav conflicts -f json`}},
	"dav conflicts list": {fields: davConflictFields, examples: []string{
		`crm dav conflicts list -f json`, `crm dav conflicts list --all -f json`}},
	"dav conflicts show": {fields: davConflictFields, examples: []string{`crm dav conflicts show 1 -f json`}},
	"dav conflicts undo": {mutates: true, fields: davConflictFields, examples: []string{`crm dav conflicts undo 1`}},

	// interfaces
	"mcp serve": {examples: []string{`crm mcp serve`}},
	"tui":       {mutates: true, noDryRun: true, examples: []string{`crm tui`, `crm tui --read-only`}},

	// agent surface and setup (added with the help system)
	"help": {examples: []string{`crm help`, `crm help duplicates`, `crm help --search merge`, `crm help overview -f json`}},
	"schema": {fields: "name,version,commands,global_flags,exit_codes,error_format,mcp_tools,help_topics", examples: []string{
		`crm schema -f json`}},
	"where": {fields: "path,source,domain,exists,schema_version,latest_schema_version,device_id,size_bytes,counts", examples: []string{
		`crm where -f json`, `crm where --db ./work.db -f json`}},
	"domains": {fields: "domain,path,schema_version,people,size_bytes,active", examples: []string{
		`crm domains -f json`, `crm domains --dir ~/.crm -f json`}},
	"init": {mutates: true, noDryRun: true, fields: "domain,path,created,imported,next_steps", examples: []string{
		`crm init --domain personal --yes -f json`, `crm init --domain work --path ~/.crm/work.db --dry-run`}},
	"keys":                 {examples: []string{`crm keys list -f json`, `crm keys set undo ctrl+z`}},
	"keys list":            {fields: keysListFields, examples: []string{`crm keys list -f json`, `crm keys list --custom`, `crm keys list --group Contact`}},
	"keys set":             {mutates: true, noDryRun: true, fields: "id,title,default,keys,saved,dry_run,config", examples: []string{`crm keys set undo ctrl+z`, `crm keys set favorite f "*" --dry-run`}},
	"keys reset":           {mutates: true, noDryRun: true, fields: "id,title,default,keys,saved,dry_run,config", examples: []string{`crm keys reset undo`, `crm keys reset --all --dry-run`}},
	"modules":              {examples: []string{`crm modules list -f json`, `crm modules off birthdays`}},
	"modules list":         {fields: modulesFields, examples: []string{`crm modules list -f json`}},
	"modules on":           {mutates: true, noDryRun: true, fields: modulesFields, examples: []string{`crm modules on deals`}},
	"modules off":          {mutates: true, noDryRun: true, fields: modulesFields, examples: []string{`crm modules off birthdays merge`}},
	"modules move":         {mutates: true, noDryRun: true, fields: modulesFields, examples: []string{`crm modules move organizations 3`}},
	"modules reset":        {mutates: true, noDryRun: true, fields: modulesFields, examples: []string{`crm modules reset`}},
	"keys path":            {fields: "config,exists", examples: []string{`crm keys path`, `crm keys path -f json`}},
	"theme":                {examples: []string{`crm theme list -f json`, `crm theme set nord`}},
	"theme list":           {fields: themeListFields, examples: []string{`crm theme list -f json`}},
	"theme set":            {mutates: true, examples: []string{`crm theme set nord`, `crm theme set my-theme --dry-run`}},
	"theme show":           {fields: themeShowFields, examples: []string{`crm theme show -f json`, `crm theme show gruvbox`}},
	"theme path":           {fields: "themes_dir,exists,config", examples: []string{`crm theme path`, `crm theme path -f json`}},
	"theme init":           {mutates: true, fields: "name,path,created", examples: []string{`crm theme init my-theme`, `crm theme init my-theme --dry-run`}},
	"doctor":               {fields: "ok,checks", examples: []string{`crm doctor -f json`}},
	"agent":                {examples: []string{`crm agent`}},
	"agent install-prompt": {examples: []string{`crm agent install-prompt`}},
	"docs":                 {examples: []string{`crm docs markdown ./docs-out`}},
	"docs markdown":        {examples: []string{`crm docs markdown ./docs-out`}},
	"docs man":             {examples: []string{`crm docs man ./man-out`}},
}

// generatedMeta describes the commands built from the object list (link, unlink, restore).
var generatedMeta = mergeMeta(linkMeta(), noteMeta())

// applyMeta annotates every command from commandMeta, adds --dry-run to mutating commands that
// lack it, and makes argument errors usage errors.
func applyMeta(root *cobra.Command) {
	for _, c := range allCommands(root) {
		path := schema.CommandPath(root, c)
		if c.Annotations == nil {
			c.Annotations = map[string]string{}
		}
		m, ok := commandMeta[path]
		if !ok {
			m, ok = generatedMeta[path]
		}
		if ok {
			c.Annotations[schema.AnnMutates] = strconv.FormatBool(m.mutates)
			c.Annotations[schema.AnnDestructive] = strconv.FormatBool(m.destructive)
			if len(m.examples) > 0 {
				c.Annotations[schema.AnnExamples] = strings.Join(m.examples, "\n")
				if c.Example == "" {
					c.Example = "  " + strings.Join(m.examples, "\n  ")
				}
			}
			if m.fields != "" {
				c.Annotations[schema.AnnOutputFields] = m.fields
			}
		}
		// The yes guard goes first so --dry-run, added last, runs before it.
		if ok && m.destructive && c.Flags().Lookup("yes") == nil {
			addYesGuard(c)
		}
		if ok && m.mutates && !m.noDryRun && c.Flags().Lookup("dry-run") == nil && c.Runnable() {
			addDryRun(c, m)
		}
		wrapArgs(c)
	}
}

func allCommands(root *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, s := range c.Commands() {
			out = append(out, s)
			walk(s)
		}
	}
	walk(root)
	return out
}

// wrapArgs turns argument-count errors into usage errors (exit code 2).
func wrapArgs(c *cobra.Command) {
	if c.Args == nil {
		return
	}
	orig := c.Args
	c.Args = func(cmd *cobra.Command, args []string) error {
		if err := orig(cmd, args); err != nil {
			return model.NewExitError(model.ErrUsage, "%s", err.Error())
		}
		return nil
	}
}

// addYesGuard requires --yes before a destructive command runs. --dry-run is checked first.
func addYesGuard(c *cobra.Command) {
	c.Flags().Bool("yes", false, "confirm: this cannot be undone")
	orig := c.RunE
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if yes, _ := cmd.Flags().GetBool("yes"); !yes {
			return model.NewExitError(model.ErrValidation, "%s cannot be undone: pass --yes to confirm (or --dry-run to preview)", schema.CommandPath(cmd.Root(), cmd))
		}
		return orig(cmd, args)
	}
}

// dryRunPlan is printed by --dry-run on commands without their own preview.
type dryRunPlan struct {
	DryRun      bool           `json:"dry_run"`
	Command     string         `json:"command"`
	Args        []string       `json:"args"`
	Flags       map[string]any `json:"flags"`
	Mutates     bool           `json:"mutates"`
	Destructive bool           `json:"destructive"`
	RequiresYes bool           `json:"requires_yes"`
	Targets     []repo.Lookup  `json:"targets"`
	Note        string         `json:"note"`
}

// addDryRun adds --dry-run. With it the command checks that its targets exist, prints the
// plan (command, arguments, flags, targets) and writes nothing.
func addDryRun(c *cobra.Command, m cmdMeta) {
	c.Flags().Bool("dry-run", false, "print what would change and write nothing")
	orig := c.RunE
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if dry, _ := cmd.Flags().GetBool("dry-run"); !dry {
			return orig(cmd, args)
		}
		return printDryRun(cmd, args, m)
	}
}

func printDryRun(cmd *cobra.Command, args []string, m cmdMeta) error {
	plan := dryRunPlan{
		DryRun: true, Command: schema.CommandPath(cmd.Root(), cmd), Args: append([]string{}, args...),
		Flags: map[string]any{}, Mutates: true, Destructive: m.destructive, RequiresYes: m.destructive,
		Targets: []repo.Lookup{}, Note: "nothing was written",
	}
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if f.Name != "dry-run" {
			plan.Flags[f.Name] = f.Value.String()
		}
	})

	refs, err := dryRunTargets(m, args)
	if err != nil {
		return err
	}
	if len(refs) > 0 {
		db, err := openDB()
		if err != nil {
			return err
		}
		defer db.Close()
		for _, ref := range refs {
			l, err := repo.LookupRecord(cmd.Context(), db, ref.entity, ref.id)
			if err != nil {
				return err
			}
			plan.Targets = append(plan.Targets, *l)
		}
	}

	if resolveFormat() == format.FormatJSON {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(plan)
	}
	fmt.Fprintf(os.Stdout, "dry run: would run: crm %s %s\n", plan.Command, strings.Join(args, " "))
	keys := make([]string, 0, len(plan.Flags))
	for k := range plan.Flags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(os.Stdout, "  --%s %v\n", k, plan.Flags[k])
	}
	for _, t := range plan.Targets {
		state := ""
		if t.Archived {
			state = " (archived)"
		}
		fmt.Fprintf(os.Stdout, "  target: %s #%d %s%s\n", t.Entity, t.ID, t.Label, state)
	}
	fmt.Fprintln(os.Stdout, "nothing was written")
	return nil
}

type recordRef struct {
	entity string
	id     int64
}

// dryRunTargets picks the record IDs out of the arguments so the preview can check they exist.
func dryRunTargets(m cmdMeta, args []string) ([]recordRef, error) {
	parse := func(s string) (int64, error) {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, model.NewExitError(model.ErrValidation, "invalid ID: %s", s)
		}
		return id, nil
	}
	var refs []recordRef
	switch m.targetArgs {
	case "first":
		if len(args) > 0 && m.target != "" {
			id, err := parse(args[0])
			if err != nil {
				return nil, err
			}
			refs = append(refs, recordRef{m.target, id})
		}
	case "all":
		for _, a := range args {
			id, err := parse(a)
			if err != nil {
				return nil, err
			}
			refs = append(refs, recordRef{m.target, id})
		}
	case "pair":
		if len(args) >= 2 && m.target != "" && m.target2 != "" {
			a, err := parse(args[0])
			if err != nil {
				return nil, err
			}
			b, err := parse(args[1])
			if err != nil {
				return nil, err
			}
			refs = append(refs, recordRef{m.target, a}, recordRef{m.target2, b})
		}
	case "tag":
		if len(args) >= 2 {
			if !model.ValidTaggable(args[0]) {
				return nil, model.NewExitError(model.ErrValidation, "invalid entity type: %s (must be one of: %s)", args[0], strings.Join(model.EntityTypes, ", "))
			}
			id, err := parse(args[1])
			if err != nil {
				return nil, err
			}
			entity := args[0]
			refs = append(refs, recordRef{entity, id})
		}
	}
	return refs, nil
}

func mergeMeta(parts ...map[string]cmdMeta) map[string]cmdMeta {
	out := map[string]cmdMeta{}
	for _, p := range parts {
		for k, v := range p {
			out[k] = v
		}
	}
	return out
}
