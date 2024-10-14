import type { LoginType, User, UserStatus } from "api/typesGenerated";

// The following username, name, and email are generated using ChatGPT to avoid
// exposing real user data in mock values. While libraries like faker.js could
// be used, static and deterministic values are preferred for snapshot tests to
// ensure consistency.
const fakeUserData = [
	{ username: "xbauer", name: "Joseph Page", email: "janet83@gmail.com" },
	{
		username: "terryjessica",
		name: "Deborah Gibson",
		email: "christianjames@yahoo.com",
	},
	{ username: "stephanie39", name: "Ivan Henry", email: "reidjohn@yahoo.com" },
	{
		username: "hweaver",
		name: "Tina Fleming",
		email: "gloriapeterson@salazar-donovan.com",
	},
	{
		username: "vhenderson",
		name: "Melissa Woods DVM",
		email: "josesalazar@jones.com",
	},
	{ username: "ronald64", name: "David Giles", email: "xlopez@tate.com" },
	{ username: "nsteele", name: "Austin Molina", email: "webbdennis@yahoo.com" },
	{
		username: "stonejonathan",
		name: "Brian Parks",
		email: "scott56@hotmail.com",
	},
	{
		username: "gary05",
		name: "Jeffrey Mosley",
		email: "matthewmyers@wheeler-butler.com",
	},
	{
		username: "oyates",
		name: "Richard Gonzalez",
		email: "sharon15@andrews-livingston.com",
	},
	{
		username: "cervantescolin",
		name: "Brian Hayes",
		email: "nataliemiller@clark.com",
	},
	{
		username: "carlosmadden",
		name: "Candace Castillo",
		email: "andrea62@schmitt-thomas.org",
	},
	{
		username: "leah46",
		name: "Mrs. Susan Murillo MD",
		email: "jamesschmitt@gmail.com",
	},
	{
		username: "eosborne",
		name: "Andrew Holland",
		email: "mileslauren@cruz.com",
	},
	{
		username: "rmorales",
		name: "Madison Shaffer",
		email: "wheeleralyssa@phillips.info",
	},
	{
		username: "haynesrachel",
		name: "Samantha Torres",
		email: "johnedwards@avery-diaz.com",
	},
	{
		username: "bakeramanda",
		name: "Michael Woods",
		email: "masseygabriel@hotmail.com",
	},
	{
		username: "gonzalesmeghan",
		name: "Tiffany Jackson",
		email: "psmith@yahoo.com",
	},
	{
		username: "xsmith",
		name: "Patrick Lewis",
		email: "christopher73@rivera.com",
	},
	{ username: "wrice", name: "Erica Smith", email: "ycisneros@hotmail.com" },
	{
		username: "anthonybrady",
		name: "Teresa Ward",
		email: "carolmartin@simmons.com",
	},
	{
		username: "matthew45",
		name: "Kevin Guerrero",
		email: "janet91@jones-brown.com",
	},
	{
		username: "brandon62",
		name: "Jennifer Jackson",
		email: "stephaniedixon@dorsey.info",
	},
	{
		username: "christinesmith",
		name: "Julian Torres",
		email: "alvarezamy@hotmail.com",
	},
	{
		username: "booneandrew",
		name: "Charles Johnson",
		email: "xblack@gmail.com",
	},
];

// These values were retrieved from the Coder API. Sensitive information such as
// usernames, names, and emails has been replaced with fake user data to protect
// privacy.
export const MockUsers: User[] = [
	{
		id: "a73425d1-53a7-43d3-b6ae-cae9ba59b92b",
		created_at: "2022-08-10T16:57:11.04414Z",
		updated_at: "2024-09-05T12:58:54.391687Z",
		last_seen_at: "2024-09-05T12:58:54.391687Z",
		status: "active",
		login_type: "github",
		theme_preference: "auto",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [
			{
				name: "owner",
				display_name: "Owner",
			},
			{
				name: "template-admin",
				display_name: "Template Admin",
			},
			{
				name: "user-admin",
				display_name: "User Admin",
			},
		],
	},
	{
		id: "350441af-b2d3-401f-a795-39971f0a682b",
		created_at: "2024-07-23T14:40:20.205142Z",
		updated_at: "2024-08-08T19:04:18.108585Z",
		last_seen_at: "2024-08-08T19:04:18.108585Z",
		status: "active",
		login_type: "github",
		theme_preference: "",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [],
	},
	{
		id: "c0240345-f14a-4632-b713-a0f09c2ed927",
		created_at: "2023-09-07T20:16:41.654648Z",
		updated_at: "2024-08-27T17:34:35.171154Z",
		last_seen_at: "2024-08-27T17:34:35.171153Z",
		status: "active",
		login_type: "password",
		theme_preference: "",
		organization_ids: [
			"703f72a1-76f6-4f89-9de6-8a3989693fe5",
			"8efa9208-656a-422d-842d-b9dec0cf1bf3",
		],
		roles: [],
	},
	{
		id: "d96bf761-3f94-46b3-a1da-6316e2e4735d",
		created_at: "2023-08-14T15:04:56.932482Z",
		updated_at: "2024-09-05T13:04:25.741671Z",
		last_seen_at: "2024-09-05T13:04:25.741671Z",
		status: "active",
		login_type: "github",
		theme_preference: "light",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [
			{
				name: "owner",
				display_name: "Owner",
			},
		],
	},
	{
		id: "27c64335-5e08-44ac-b93f-e454b82a9a06",
		created_at: "2024-06-10T18:10:33.595496Z",
		updated_at: "2024-07-25T22:39:21.144268Z",
		last_seen_at: "2024-07-25T22:39:21.144268Z",
		status: "active",
		login_type: "oidc",
		theme_preference: "",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [],
	},
	{
		id: "0f452b63-64cb-4422-99ea-7391ccf7b4d5",
		created_at: "2024-06-20T15:31:39.835721Z",
		updated_at: "2024-06-20T15:31:39.916055Z",
		last_seen_at: "2024-06-20T15:31:39.916055Z",
		status: "active",
		login_type: "github",
		theme_preference: "",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [],
	},
	{
		id: "135e83f1-ff91-4f91-8d23-f7e7fa6627f1",
		created_at: "2024-07-18T16:56:04.513102Z",
		updated_at: "2024-09-05T02:37:40.678649Z",
		last_seen_at: "2024-09-05T02:37:40.678649Z",
		status: "active",
		login_type: "github",
		theme_preference: "",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [],
	},
	{
		id: "9e3e4c5a-5949-417f-9380-d0a393c78bdd",
		created_at: "2024-07-15T02:00:51.816307Z",
		updated_at: "2024-09-05T04:32:12.923203Z",
		last_seen_at: "2024-09-05T04:32:12.923203Z",
		status: "active",
		login_type: "oidc",
		theme_preference: "",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [
			{
				name: "owner",
				display_name: "Owner",
			},
		],
	},
	{
		id: "58af7daf-5d08-456e-ae3b-5fba07b3df80",
		created_at: "2024-07-22T20:31:19.143974Z",
		updated_at: "2024-07-25T13:21:14.248194Z",
		last_seen_at: "2024-07-25T13:21:14.248194Z",
		status: "active",
		login_type: "password",
		theme_preference: "",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [],
	},
	{
		id: "5ccd3128-cbbb-4cfb-8139-5a1edbb60c71",
		created_at: "2022-08-10T20:33:51.324299Z",
		updated_at: "2024-09-05T13:00:02.205822Z",
		last_seen_at: "2024-09-05T13:00:02.205822Z",
		status: "active",
		login_type: "github",
		theme_preference: "dark",
		organization_ids: [
			"cbdcf774-9412-4118-8cd9-b3f502c84dfb",
			"703f72a1-76f6-4f89-9de6-8a3989693fe5",
			"7621bbb4-5b04-4957-8419-cf4a683ac59a",
		],
		roles: [
			{
				name: "user-admin",
				display_name: "User Admin",
			},
			{
				name: "template-admin",
				display_name: "Template Admin",
			},
			{
				name: "auditor",
				display_name: "Auditor",
			},
			{
				name: "owner",
				display_name: "Owner",
			},
		],
	},
	{
		id: "af657bc3-6949-4b1b-bc2d-d41a40b546a4",
		created_at: "2022-08-10T16:35:11.879233Z",
		updated_at: "2024-09-05T13:12:27.319427Z",
		last_seen_at: "2024-09-05T13:12:27.319427Z",
		status: "active",
		login_type: "github",
		theme_preference: "dark",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [
			{
				name: "owner",
				display_name: "Owner",
			},
			{
				name: "user-admin",
				display_name: "User Admin",
			},
		],
	},
	{
		id: "1c2baba2-40d6-4b9e-a788-fe393c1dbdbb",
		created_at: "2023-01-03T13:29:52.76039Z",
		updated_at: "2024-09-03T16:40:45.784352Z",
		last_seen_at: "2024-09-03T16:40:45.784352Z",
		status: "active",
		login_type: "password",
		theme_preference: "dark",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [
			{
				name: "owner",
				display_name: "Owner",
			},
		],
	},
	{
		id: "f2cdaba3-e0a3-447a-b186-eb10dc1d1d49",
		created_at: "2024-02-02T20:37:29.606054Z",
		updated_at: "2024-06-10T14:31:49.820912Z",
		last_seen_at: "2024-06-10T14:31:49.820912Z",
		status: "active",
		login_type: "password",
		theme_preference: "",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [],
	},
	{
		id: "9bc756d1-5e95-4c6f-8e1b-a1bd20547151",
		created_at: "2024-09-04T11:29:24.168944Z",
		updated_at: "2024-09-04T11:29:24.658926Z",
		last_seen_at: "2024-09-04T11:29:24.658926Z",
		status: "active",
		login_type: "oidc",
		theme_preference: "",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [
			{
				name: "owner",
				display_name: "Owner",
			},
		],
	},
	{
		id: "59da0bfe-9c99-47fa-a563-f9fdb18449d0",
		created_at: "2022-08-15T08:30:10.343828Z",
		updated_at: "2024-09-05T12:27:22.098297Z",
		last_seen_at: "2024-09-05T12:27:22.098297Z",
		status: "active",
		login_type: "oidc",
		theme_preference: "dark",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [
			{
				name: "owner",
				display_name: "Owner",
			},
		],
	},
	{
		id: "12b03f43-1bb7-4fca-967a-585c97f31682",
		created_at: "2022-08-10T15:35:20.553581Z",
		updated_at: "2024-09-05T13:23:46.237798Z",
		last_seen_at: "2024-09-05T13:23:46.237798Z",
		status: "active",
		login_type: "github",
		theme_preference: "dark",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [
			{
				name: "owner",
				display_name: "Owner",
			},
			{
				name: "template-admin",
				display_name: "Template Admin",
			},
			{
				name: "user-admin",
				display_name: "User Admin",
			},
		],
	},
	{
		id: "78dd2361-4a5a-42b0-9ec3-3eea23af1094",
		created_at: "2022-08-10T17:14:30.475925Z",
		updated_at: "2024-09-04T20:40:17.036986Z",
		last_seen_at: "2024-09-04T20:40:17.036986Z",
		status: "active",
		login_type: "github",
		theme_preference: "auto",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [
			{
				name: "owner",
				display_name: "Owner",
			},
			{
				name: "template-admin",
				display_name: "Template Admin",
			},
			{
				name: "user-admin",
				display_name: "User Admin",
			},
		],
	},
	{
		id: "4a44ccb6-196b-4d98-a97d-8338bb751fc0",
		created_at: "2024-08-26T09:00:17.565927Z",
		updated_at: "2024-09-05T12:45:45.987041Z",
		last_seen_at: "2024-09-05T12:45:45.987041Z",
		status: "active",
		login_type: "github",
		theme_preference: "auto",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [
			{
				name: "owner",
				display_name: "Owner",
			},
		],
	},
	{
		id: "c323e5c3-57cb-45e7-81c4-56d6cacb2f8c",
		created_at: "2024-03-04T11:12:41.201352Z",
		updated_at: "2024-09-05T07:24:39.32465Z",
		last_seen_at: "2024-09-05T07:24:39.324649Z",
		status: "active",
		login_type: "oidc",
		theme_preference: "dark",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [
			{
				name: "owner",
				display_name: "Owner",
			},
		],
	},
	{
		id: "9e7815af-bc48-435a-91c7-72dcbf26f036",
		created_at: "2024-05-24T14:53:53.996555Z",
		updated_at: "2024-07-22T16:43:16.494533Z",
		last_seen_at: "2024-07-22T16:43:16.494533Z",
		status: "active",
		login_type: "oidc",
		theme_preference: "",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [],
	},
	{
		id: "3f8c0eef-6a45-4759-a4d6-d00bbffb1369",
		created_at: "2022-08-15T12:31:15.833843Z",
		updated_at: "2024-09-05T09:21:46.442564Z",
		last_seen_at: "2024-09-05T09:21:46.442564Z",
		status: "active",
		login_type: "github",
		theme_preference: "",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [
			{
				name: "owner",
				display_name: "Owner",
			},
			{
				name: "template-admin",
				display_name: "Template Admin",
			},
			{
				name: "user-admin",
				display_name: "User Admin",
			},
		],
	},
	{
		id: "c5eb8310-cf4f-444c-b223-0e991f828b40",
		created_at: "2022-08-10T20:00:05.494466Z",
		updated_at: "2024-09-04T18:33:12.702949Z",
		last_seen_at: "2024-09-04T18:33:12.702948Z",
		status: "active",
		login_type: "github",
		theme_preference: "dark",
		organization_ids: [
			"703f72a1-76f6-4f89-9de6-8a3989693fe5",
			"8efa9208-656a-422d-842d-b9dec0cf1bf3",
		],
		roles: [
			{
				name: "template-admin",
				display_name: "Template Admin",
			},
			{
				name: "user-admin",
				display_name: "User Admin",
			},
			{
				name: "auditor",
				display_name: "Auditor",
			},
			{
				name: "owner",
				display_name: "Owner",
			},
		],
	},
	{
		id: "740bba7f-356d-4203-8f15-03ddee381998",
		created_at: "2022-08-10T18:13:52.084503Z",
		updated_at: "2024-09-05T11:56:07.949264Z",
		last_seen_at: "2024-09-05T11:56:07.949264Z",
		status: "active",
		login_type: "github",
		theme_preference: "dark",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [
			{
				name: "owner",
				display_name: "Owner",
			},
			{
				name: "template-admin",
				display_name: "Template Admin",
			},
			{
				name: "user-admin",
				display_name: "User Admin",
			},
		],
	},
	{
		id: "806e35bb-37fd-4810-b5f2-88aa47c30c84",
		created_at: "2024-06-03T03:09:52.16976Z",
		updated_at: "2024-09-05T12:51:39.933117Z",
		last_seen_at: "2024-09-05T12:51:39.933117Z",
		status: "active",
		login_type: "oidc",
		theme_preference: "dark",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [
			{
				name: "user-admin",
				display_name: "User Admin",
			},
			{
				name: "template-admin",
				display_name: "Template Admin",
			},
			{
				name: "auditor",
				display_name: "Auditor",
			},
			{
				name: "owner",
				display_name: "Owner",
			},
		],
	},
	{
		id: "3b0d7f28-7ec0-4d0a-b634-57af9d739e06",
		created_at: "2024-07-08T12:31:40.932658Z",
		updated_at: "2024-07-12T06:33:22.019268Z",
		last_seen_at: "2024-07-12T06:33:22.019268Z",
		status: "active",
		login_type: "github",
		theme_preference: "",
		organization_ids: ["703f72a1-76f6-4f89-9de6-8a3989693fe5"],
		roles: [],
	},
].map((u, i) => ({
	...u,
	...fakeUserData[i],
	avatar_url: "",
	status: u.status as UserStatus,
	login_type: u.login_type as LoginType,
}));
