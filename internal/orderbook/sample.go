package orderbook

// Market is which worked example to load.
type Market string

const (
	// IN is the Indian worked example.
	IN Market = "IN"
	// PH is the Philippine worked example.
	PH Market = "PH"
)

// Markets are both worked examples.
func Markets() []Market { return []Market{IN, PH} }

// MarketName is the market written out.
func MarketName(m Market) string {
	if m == PH {
		return "the Philippines"
	}
	return "India"
}

// Sample is a worked shop: a catalogue, a book with orders at several stages,
// and a day with enough on it to be worth reading. It is there so that the
// whole tool can be seen working before anything real is typed into it.
//
// Every name, number and address in it is invented. The numbers are in the
// ranges reserved for fiction where there is one, and none of them is anybody's
// number.
func Sample(m Market, today Date) (Shop, Catalogue, []Order) {
	if m == PH {
		return samplePH(today)
	}
	return sampleIN(today)
}

func sampleIN(today Date) (Shop, Catalogue, []Order) {
	shop := Shop{
		Name:       "Aaji's Kitchen",
		Area:       "Kothrud, Pune",
		Currency:   INR,
		DialCode:   "91",
		Phone:      "+91 98765 43210",
		Email:      "orders@aajiskitchen.example",
		PayTo:      "UPI aajiskitchen@examplebank, or cash on delivery",
		HowToOrder: "Reply with the item and how many, and the day you want it. Orders by 8pm for the next day.",
		Closing:    "Everything is made at home, in small batches.",
		Limits:     DefaultLimits(),
	}.Normalise()

	catalogue := Catalogue{
		PricesChangedOn: today.AddDays(-9),
		Products: []Product{
			{
				Code: "MANGO-500", Name: "Mango pickle", Unit: "jar",
				Price: 32000, PackAmount: Whole(500), PackMeasure: ByWeight,
				Availability: Available, Note: "no preservatives",
			},
			{
				Code: "MANGO-1000", Name: "Mango pickle", Unit: "jar",
				Price: 58000, PackAmount: Whole(1000), PackMeasure: ByWeight,
				Availability: Available, Note: "the family jar",
			},
			{
				Code: "LIME-400", Name: "Lime pickle", Unit: "jar",
				Price: 26000, PackAmount: Whole(400), PackMeasure: ByWeight,
				Availability: Available,
			},
			{
				Code: "CHAKLI-250", Name: "Chakli", Unit: "packet",
				Price: 14000, PackAmount: Whole(250), PackMeasure: ByWeight,
				Availability: MadeToOrder, NoticeDays: 2, MinQty: Whole(2),
				Note: "fried the morning it goes out",
			},
			{
				Code: "LADDU-12", Name: "Besan laddu", Unit: "box",
				Price: 48000, PackAmount: Whole(12), PackMeasure: ByCount,
				Availability: MadeToOrder, NoticeDays: 1,
			},
			{
				Code: "PODI-200", Name: "Idli podi", Unit: "packet",
				Price: 18000, PackAmount: Whole(200), PackMeasure: ByWeight,
				Availability: SoldOut, Note: "back next week",
			},
			{
				Code: "HAMPER-FEST", Name: "Festival hamper", Unit: "hamper",
				Price: 125000, Availability: MadeToOrder, NoticeDays: 4,
				Note: "four jars and two boxes, made up to order", Hidden: true,
			},
		},
	}.Normalise()

	tomorrow := today.AddDays(1)
	yesterday := today.AddDays(-1)

	orders := []Order{
		{
			Ref:      "ORD-0001",
			PlacedOn: today.AddDays(-4),
			Customer: Customer{
				Name: "Meera Kulkarni", Phone: "098765 43211",
				Address: "Flat 3, Shivam Apartments, Lane 6, Kothrud",
			},
			Fulfilment: Delivery, Wanted: yesterday, Slot: SlotMorning,
			Lines: []Line{
				{Code: "MANGO-500", Name: "Mango pickle", Pack: "500 g", Unit: "jar", Qty: Whole(2), UnitPrice: 30000},
				{Code: "LIME-400", Name: "Lime pickle", Pack: "400 g", Unit: "jar", Qty: Whole(1), UnitPrice: 26000},
			},
			DeliveryFee: 4000,
			Paid:        9000, PaymentNote: "part by UPI",
			Status: Delivered,
			Note:   "Leave with the watchman if nobody is in.",
		},
		{
			Ref:      "ORD-0002",
			PlacedOn: today.AddDays(-2),
			Customer: Customer{
				Name: "Sandeep Rao", Phone: "+91 98765 43212",
				Address: "22 Bhandarkar Road, Pune",
			},
			Fulfilment: Delivery, Wanted: tomorrow, Slot: SlotEvening,
			Lines: []Line{
				{Code: "CHAKLI-250", Name: "Chakli", Pack: "250 g", Unit: "packet", Qty: Whole(4), UnitPrice: 14000, Note: "less chilli"},
				{Code: "LADDU-12", Name: "Besan laddu", Pack: "12 pieces", Unit: "box", Qty: Whole(1), UnitPrice: 48000},
			},
			DeliveryFee: 4000, Discount: 5000, DiscountReason: "regular customer",
			Paid: 50000, PaymentNote: "UPI, advance",
			Status: Confirmed,
		},
		{
			Ref:      "ORD-0003",
			PlacedOn: today,
			Customer: Customer{
				Name: "Priya Deshmukh", Phone: "9876543213",
			},
			Fulfilment: Pickup, Wanted: tomorrow, Slot: SlotMorning,
			Lines: []Line{
				{Code: "MANGO-1000", Name: "Mango pickle", Pack: "1 kg", Unit: "jar", Qty: Whole(1), UnitPrice: 58000},
				{Code: "CHAKLI-250", Name: "Chakli", Pack: "250 g", Unit: "packet", Qty: Whole(2), UnitPrice: 14000},
			},
			Status: Taken,
			Note:   "Collecting on the way to the station.",
		},
		{
			Ref:      "ORD-0004",
			PlacedOn: today.AddDays(-1),
			Customer: Customer{
				Name: "Anil Joshi", Phone: "098765 43214",
				Address: "Office 4, Paud Road, Pune",
			},
			Fulfilment: Delivery, Wanted: today, Slot: SlotAfternoon,
			Lines: []Line{
				{Code: "LIME-400", Name: "Lime pickle", Pack: "400 g", Unit: "jar", Qty: Whole(3), UnitPrice: 26000},
			},
			DeliveryFee: 4000,
			Status:      Cancelled,
			Note:        "Cancelled by the customer, travelling.",
		},
	}
	for i := range orders {
		orders[i] = orders[i].Normalise()
	}
	return shop, catalogue, orders
}

func samplePH(today Date) (Shop, Catalogue, []Order) {
	shop := Shop{
		Name:       "Tita Baking Co.",
		Area:       "Project 4, Quezon City",
		Currency:   PHP,
		DialCode:   "63",
		Phone:      "0917 123 4567",
		Email:      "orders@titabaking.example",
		PayTo:      "GCash 0917 123 4567, or cash on pickup",
		HowToOrder: "Reply with the item and how many, and the day you need it. Orders by 6pm for the next day.",
		Closing:    "Baked fresh the morning it goes out.",
		Limits:     DefaultLimits(),
	}.Normalise()

	catalogue := Catalogue{
		PricesChangedOn: today.AddDays(-6),
		Products: []Product{
			{
				Code: "PANDESAL-12", Name: "Ube cheese pandesal", Unit: "dozen",
				Price: 18000, PackAmount: Whole(12), PackMeasure: ByCount,
				Availability: Available, Note: "best on the day",
			},
			{
				Code: "BANANA-LOAF", Name: "Banana bread", Unit: "loaf",
				Price: 25000, PackAmount: Whole(500), PackMeasure: ByWeight,
				Availability: Available,
			},
			{
				Code: "FLAN-8", Name: "Leche flan", Unit: "tray",
				Price: 32000, PackAmount: Whole(8), PackMeasure: ByCount,
				Availability: MadeToOrder, NoticeDays: 1,
				Note: "eight slices",
			},
			{
				Code: "CALAMANSI-250", Name: "Calamansi jam", Unit: "jar",
				Price: 15000, PackAmount: Whole(250), PackMeasure: ByVolume,
				Availability: Available,
			},
			{
				Code: "GRAHAM-BALLS", Name: "Graham balls", Unit: "box",
				Price: 12000, PackAmount: Whole(20), PackMeasure: ByCount,
				Availability: SoldOut, Note: "back on Friday",
			},
			{
				Code: "CAKE-ORDER", Name: "Birthday cake", Unit: "cake",
				Price: 95000, Availability: MadeToOrder, NoticeDays: 3,
				Note: "message for sizes and flavours",
			},
		},
	}.Normalise()

	tomorrow := today.AddDays(1)

	orders := []Order{
		{
			Ref:      "ORD-0001",
			PlacedOn: today.AddDays(-3),
			Customer: Customer{
				Name: "Liza Mendoza", Phone: "0917 123 4568",
				Address: "14 Maginhawa St, Teachers Village, Quezon City",
			},
			Fulfilment: Delivery, Wanted: today, Slot: SlotMorning,
			Lines: []Line{
				{Code: "PANDESAL-12", Name: "Ube cheese pandesal", Pack: "12 pieces", Unit: "dozen", Qty: Whole(3), UnitPrice: 17000},
				{Code: "CALAMANSI-250", Name: "Calamansi jam", Pack: "250 ml", Unit: "jar", Qty: Whole(2), UnitPrice: 15000},
			},
			DeliveryFee: 6000,
			Paid:        8700, PaymentNote: "GCash, half now",
			Status: Packed,
		},
		{
			Ref:      "ORD-0002",
			PlacedOn: today.AddDays(-1),
			Customer: Customer{
				Name: "Jun Ramos", Phone: "+63 917 123 4569",
			},
			Fulfilment: Pickup, Wanted: tomorrow, Slot: SlotAfternoon,
			Lines: []Line{
				{Code: "FLAN-8", Name: "Leche flan", Pack: "8 pieces", Unit: "tray", Qty: Whole(2), UnitPrice: 32000},
				{Code: "BANANA-LOAF", Name: "Banana bread", Pack: "500 g", Unit: "loaf", Qty: Whole(1), UnitPrice: 25000},
			},
			Status: Confirmed,
			Note:   "For the office merienda.",
		},
		{
			Ref:      "ORD-0003",
			PlacedOn: today,
			Customer: Customer{
				Name: "Ate Nena", Phone: "09171234570",
				Address: "Blk 7 Lot 2, Bagong Pag-asa, Quezon City",
			},
			Fulfilment: Delivery, Wanted: today, Slot: SlotEvening,
			Lines: []Line{
				{Code: "PANDESAL-12", Name: "Ube cheese pandesal", Pack: "12 pieces", Unit: "dozen", Qty: Whole(2), UnitPrice: 18000},
			},
			DeliveryFee: 6000,
			Status:      OutForDelivery,
		},
	}
	for i := range orders {
		orders[i] = orders[i].Normalise()
	}
	return shop, catalogue, orders
}
