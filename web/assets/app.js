/* The local interface to the catalogue and order book.
   It talks only to /api on this same loopback address, and every figure on
   screen is worked out by the binary — this file draws, it does not decide. */
(function () {
  "use strict";

  var $ = function (id) { return document.getElementById(id); };
  var state = null;
  var pickedOrder = null;
  var editingProduct = null;
  var draftLines = [];

  /* ───────── theme ───────── */
  (function theme() {
    var root = document.documentElement;
    var button = document.querySelector("[data-themer]");
    if (!button) { return; }
    function stored() { try { return localStorage.getItem("omsme-theme"); } catch (e) { return null; } }
    function current() {
      var set = root.getAttribute("data-theme");
      if (set) { return set; }
      return (window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches) ? "dark" : "light";
    }
    function paint() {
      var dark = current() === "dark";
      button.textContent = dark ? "☀ Light" : "☾ Dark";
      button.setAttribute("aria-label", dark ? "Switch to light theme" : "Switch to dark theme");
    }
    paint();
    button.addEventListener("click", function () {
      var next = current() === "dark" ? "light" : "dark";
      root.setAttribute("data-theme", next);
      try { localStorage.setItem("omsme-theme", next); } catch (e) { /* private window */ }
      paint();
    });
    if (window.matchMedia) {
      window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", function () {
        if (!stored()) { paint(); }
      });
    }
  }());

  /* ───────── tabs ───────── */
  var PANELS = ["today", "orders", "new", "catalogue", "share", "shop"];
  var tabs = Array.prototype.slice.call(document.querySelectorAll("[data-tab]"));
  tabs.forEach(function (button) {
    button.addEventListener("click", function () { show(button.getAttribute("data-tab")); });
  });
  function show(name) {
    tabs.forEach(function (button) {
      button.setAttribute("aria-pressed", button.getAttribute("data-tab") === name ? "true" : "false");
    });
    PANELS.forEach(function (panel) { $("panel-" + panel).hidden = panel !== name; });
    try { localStorage.setItem("cob-tab", name); } catch (e) { /* private window */ }
    if (name === "share" && state) { refreshShare(); }
  }

  /* ───────── plumbing ───────── */
  function flash(message, level) {
    var box = $("flash");
    box.textContent = message || "";
    box.hidden = !message;
    if (level) { box.setAttribute("data-level", level); } else { box.removeAttribute("data-level"); }
  }

  function busy(message) { $("saving").textContent = message || ""; }

  function api(method, path, body) {
    var options = { method: method, headers: { "Accept": "application/json" } };
    if (body !== undefined) {
      options.headers["Content-Type"] = "application/json";
      options.body = JSON.stringify(body);
    }
    busy("working…");
    return fetch(path, options).then(function (response) {
      return response.text().then(function (text) {
        busy("");
        var parsed = null;
        try { parsed = text ? JSON.parse(text) : {}; } catch (e) { parsed = { error: text }; }
        if (!response.ok) { throw new Error((parsed && parsed.error) || ("the request failed: " + response.status)); }
        return parsed;
      });
    }, function (err) { busy(""); throw err; });
  }

  function text(node, value) { node.textContent = (value === undefined || value === null) ? "" : String(value); }

  function cell(row, value, className) {
    var td = document.createElement("td");
    if (className) { td.className = className; }
    if (value instanceof Node) { td.appendChild(value); } else { text(td, value); }
    row.appendChild(td);
    return td;
  }

  function headCell(row, value, className) {
    var th = document.createElement("th");
    if (className) { th.className = className; }
    text(th, value);
    row.appendChild(th);
    return th;
  }

  function button(label, className, onClick) {
    var node = document.createElement("button");
    node.type = "button";
    if (className) { node.className = className; }
    text(node, label);
    node.addEventListener("click", onClick);
    return node;
  }

  function chip(label, band) {
    var span = document.createElement("span");
    span.className = "chip" + (band ? " " + band : "");
    text(span, label);
    return span;
  }

  function stack(main, meta) {
    var box = document.createElement("div");
    var strong = document.createElement("span");
    strong.className = "prod";
    text(strong, main);
    box.appendChild(strong);
    if (meta) {
      var small = document.createElement("span");
      small.className = "meta";
      text(small, meta);
      box.appendChild(small);
    }
    return box;
  }

  function empty(node) { while (node.firstChild) { node.removeChild(node.firstChild); } }

  function emptyRow(body, message, span) {
    var row = document.createElement("tr");
    var td = cell(row, message);
    td.colSpan = span || 4;
    td.className = "meta";
    body.appendChild(row);
  }

  function options(select, list, valueOf, labelOf, selected) {
    empty(select);
    list.forEach(function (item) {
      var option = document.createElement("option");
      option.value = valueOf(item);
      text(option, labelOf(item));
      if (String(valueOf(item)) === String(selected)) { option.selected = true; }
      select.appendChild(option);
    });
  }

  /* Money and quantities come from the tool as decimal strings. The page shows
     them with the shop's own sign and never does arithmetic on them: every
     figure on screen was worked out by the binary. */
  function money(value) {
    if (value === undefined || value === null || value === "") { return "—"; }
    var sign = (state && state.shop && state.shop.currency === "PHP") ? "₱" : "₹";
    var negative = String(value).charAt(0) === "-";
    var digits = negative ? String(value).slice(1) : String(value);
    var parts = digits.split(".");
    var whole = parts[0];
    var minor = parts.length > 1 ? parts[1] : "00";
    return (negative ? "-" : "") + sign + group(whole) + "." + minor;
  }

  function group(digits) {
    if (digits.length <= 3) { return digits; }
    var indian = !(state && state.shop && state.shop.currency === "PHP");
    var head = digits.slice(0, digits.length - 3);
    var tail = digits.slice(digits.length - 3);
    var size = indian ? 2 : 3;
    var parts = [];
    while (head.length > size) {
      parts.unshift(head.slice(head.length - size));
      head = head.slice(0, head.length - size);
    }
    parts.unshift(head);
    return parts.join(",") + "," + tail;
  }

  function problems(node, list, noneMessage) {
    empty(node);
    if (!list || !list.length) {
      var fine = document.createElement("li");
      fine.className = "none";
      text(fine, noneMessage || "Nothing to look at.");
      node.appendChild(fine);
      return;
    }
    list.forEach(function (problem) {
      var item = document.createElement("li");
      item.setAttribute("data-severity", problem.severity);
      var code = document.createElement("span");
      code.className = "code";
      text(code, problem.severity === "stop" ? "must be fixed · " + problem.code : "look at · " + problem.code);
      item.appendChild(code);
      item.appendChild(document.createTextNode(problem.message));
      node.appendChild(item);
    });
  }

  /* A link block: the chat link if there is one, the mail link if there is one,
     and always the text to copy — because a link is never the only way out. */
  function linkRow(container, link, label) {
    if (!link) { return; }
    if (link.url) {
      var anchor = document.createElement("a");
      anchor.className = "btn " + (link.safe ? "primary" : "ghost");
      anchor.href = link.url;
      anchor.target = "_blank";
      anchor.rel = "noopener noreferrer";
      text(anchor, label);
      container.appendChild(anchor);
    }
    if (link.text) {
      container.appendChild(button("Copy the message", "btn ghost", function () {
        copy(link.text);
      }));
    }
  }

  function copy(value) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(value).then(function () {
        flash("Copied. Paste it into the chat.", "ok");
      }, function () { flash("This browser would not copy it. Select the text and copy it by hand.", "bad"); });
      return;
    }
    flash("This browser has no clipboard. Select the text and copy it by hand.", "bad");
  }

  function linkMeta(container, link) {
    if (!link) { return; }
    var note = document.createElement("p");
    note.className = "linkmeta";
    var bits = [];
    if (link.textCharacters) { bits.push(link.textCharacters + " characters of message"); }
    if (link.characters) { bits.push(link.characters + " characters of link"); }
    if (link.note) { bits.push(link.note); }
    text(note, bits.join(" · "));
    container.appendChild(note);
  }

  /* ───────── today ───────── */
  function drawToday() {
    var sheet = state.daySheet;
    $("day-on").value = sheet.on || state.today;

    text($("day-collect"), money(sheet.totals.toCollect));
    text($("day-sub"), sheet.pretty + " · " + sheet.totals.orders + " order(s) worth " +
      money(sheet.totals.value) + (sheet.totals.cancelled ? " · " + sheet.totals.cancelled + " cancelled" : ""));

    var make = $("day-make");
    empty(make);
    if (!sheet.make.length) {
      emptyRow(make, "Nothing to make on this day.", 3);
    } else {
      sheet.make.forEach(function (line) {
        var row = document.createElement("tr");
        var label = line.name + (line.pack ? " (" + line.pack + ")" : "");
        var holder = stack(label, line.soldOut ? "marked sold out in the catalogue" : "");
        cell(row, holder);
        cell(row, line.qty, "num");
        cell(row, line.orders, "num");
        make.appendChild(row);
      });
    }

    var going = $("day-orders");
    empty(going);
    var rows = sheet.orders.concat(sheet.cancelled);
    if (!rows.length) {
      emptyRow(going, "Nothing going out on this day.", 4);
    } else {
      rows.forEach(function (row) {
        var tr = document.createElement("tr");
        cell(tr, row.slotName);
        var who = stack(row.customer, row.where + " · " + row.ref);
        if (row.status === "cancelled") { who.appendChild(chip("cancelled", "bad")); }
        cell(tr, who);
        cell(tr, money(row.total), "num");
        cell(tr, row.balance === "0.00" ? "paid" : money(row.balance), "num");
        going.appendChild(tr);
      });
    }

    $("dl-day").href = "/api/day.csv?on=" + encodeURIComponent(sheet.on || state.today);

    var book = $("book-table").querySelector("tbody");
    empty(book);
    [
      ["Orders in the book", state.book.orders],
      ["Still to do", state.book.open],
      ["Day already past", state.book.overdue],
      ["No day set", state.book.unscheduled],
      ["Needing attention", state.book.withProblems],
      ["Worth", money(state.book.value)],
      ["Collected", money(state.book.collected)],
      ["Still to collect", money(state.book.toCollect)]
    ].forEach(function (pair) {
      var row = document.createElement("tr");
      headCell(row, pair[0]);
      cell(row, pair[1], "num");
      book.appendChild(row);
    });

    var days = $("book-days");
    empty(days);
    if (!state.book.days.length) {
      var none = document.createElement("li");
      text(none, "No days with work on them.");
      days.appendChild(none);
    } else {
      state.book.days.forEach(function (day) {
        var item = document.createElement("li");
        if (day.past) { item.className = "past"; }
        var when = document.createElement("span");
        when.className = "when";
        text(when, day.pretty + (day.past ? " — already past" : ""));
        var what = document.createElement("span");
        what.className = "what";
        text(what, day.orders + " order(s) · " + money(day.toCollect));
        item.appendChild(when);
        item.appendChild(what);
        item.addEventListener("click", function () { loadDay(day.on); });
        days.appendChild(item);
      });
    }

    text($("day-text"), state.dayText);
    var links = $("day-links");
    empty(links);
    api("GET", "/api/day?on=" + encodeURIComponent(sheet.on || state.today)).then(function (answer) {
      empty(links);
      linkRow(links, answer.message.whatsapp, "Send it to myself");
      linkMeta(links, answer.message.whatsapp);
      var csv = document.createElement("a");
      csv.className = "btn ghost";
      csv.href = "/api/day.csv?on=" + encodeURIComponent(sheet.on || state.today);
      text(csv, "As CSV");
      links.appendChild(csv);
    }, function (err) { flash(err.message, "bad"); });
  }

  function loadDay(on) {
    return api("GET", "/api/state?on=" + encodeURIComponent(on)).then(function (next) {
      state = next;
      drawToday();
    }, function (err) { flash(err.message, "bad"); });
  }

  $("day-on").addEventListener("change", function () {
    if (this.value) { loadDay(this.value); }
  });

  /* ───────── orders ───────── */
  function drawOrders() {
    var filter = $("order-filter");
    if (!filter.options.length) {
      options(filter, [{ v: "all", n: "every order" }, { v: "open", n: "still to do" },
        { v: "problems", n: "needing attention" }, { v: "owing", n: "with money owing" }],
        function (o) { return o.v; }, function (o) { return o.n; }, "open");
    }

    var want = filter.value || "open";
    var body = $("order-rows");
    empty(body);

    var shown = state.orders.filter(function (view) {
      if (want === "open") { return view.order.status !== "delivered" && view.order.status !== "cancelled"; }
      if (want === "problems") { return view.blocked; }
      if (want === "owing") { return view.totals.balance !== "0.00" && view.order.status !== "cancelled"; }
      return true;
    });

    if (!shown.length) {
      emptyRow(body, "No orders to show.", 5);
      return;
    }

    shown.forEach(function (view) {
      var row = document.createElement("tr");
      if (pickedOrder === view.order.ref) { row.className = "picked"; }

      var pick = button(view.order.ref, "link", function () { openOrder(view.order.ref); });
      var holder = document.createElement("div");
      holder.appendChild(pick);
      var meta = document.createElement("span");
      meta.className = "meta";
      text(meta, view.order.customer.name || "no name");
      holder.appendChild(meta);
      cell(row, holder);

      cell(row, view.order.wanted ? view.order.wanted + " " + (view.order.slot || "") : "no day set");
      cell(row, money(view.totals.total), "num");
      cell(row, view.totals.balance === "0.00" ? "paid" : money(view.totals.balance), "num");

      var stage = document.createElement("div");
      stage.appendChild(chip(view.order.status, view.order.status === "cancelled" ? "bad" :
        (view.order.status === "delivered" ? "ok" : "")));
      if (view.blocked) { stage.appendChild(chip("fix first", "bad")); }
      cell(row, stage);

      body.appendChild(row);
    });
  }

  $("order-filter").addEventListener("change", drawOrders);

  function openOrder(ref) {
    pickedOrder = ref;
    drawOrders();
    return api("GET", "/api/order/messages?ref=" + encodeURIComponent(ref)).then(function (answer) {
      drawOrderDetail(answer.order, answer.messages);
    }, function (err) { flash(err.message, "bad"); });
  }

  function drawOrderDetail(view, messages) {
    text($("order-title"), view.order.ref + " — " + (view.order.customer.name || "no name"));
    var box = $("order-detail");
    empty(box);

    [["Wanted", view.order.wanted ? view.order.wanted + ", " + (view.order.slot || "any time") : "no day set"],
      ["Taken on", view.order.placedOn],
      ["Earliest possible", view.earliest],
      ["Way", view.order.fulfilment],
      ["Phone", view.order.customer.phone || "none"],
      ["Address", view.order.customer.address || "none"]].forEach(function (pair) {
      var row = document.createElement("div");
      row.className = "detailrow";
      var k = document.createElement("span");
      k.className = "k";
      text(k, pair[0]);
      var v = document.createElement("span");
      v.className = "v";
      text(v, pair[1]);
      row.appendChild(k);
      row.appendChild(v);
      box.appendChild(row);
    });

    var table = document.createElement("table");
    table.className = "breakdown";
    var body = document.createElement("tbody");
    view.order.lines.forEach(function (line, index) {
      var row = document.createElement("tr");
      cell(row, stack(line.name + (line.pack ? " (" + line.pack + ")" : ""),
        line.qty + " × " + money(line.unitPrice) + (line.note ? " · " + line.note : "")));
      // The figure the tool worked out, not one this page multiplied.
      cell(row, money(view.lineAmounts[index]), "num");
      body.appendChild(row);
    });
    [["Items", view.totals.subtotal], ["Delivery", view.totals.deliveryFee],
      ["Discount", view.totals.discount]].forEach(function (pair) {
      if (pair[1] === "0.00") { return; }
      var row = document.createElement("tr");
      row.className = "sub";
      headCell(row, pair[0]);
      cell(row, money(pair[1]), "num");
      body.appendChild(row);
    });
    var totalRow = document.createElement("tr");
    totalRow.className = "total";
    headCell(totalRow, "Total");
    cell(totalRow, money(view.totals.total), "num");
    body.appendChild(totalRow);

    var paidRow = document.createElement("tr");
    headCell(paidRow, "Received");
    cell(paidRow, money(view.totals.paid), "num");
    body.appendChild(paidRow);
    var owedRow = document.createElement("tr");
    headCell(owedRow, "Still to pay");
    cell(owedRow, money(view.totals.balance), "num");
    body.appendChild(owedRow);

    table.appendChild(body);
    box.appendChild(table);

    /* Money received. */
    var payWrap = document.createElement("div");
    payWrap.className = "field";
    var payLabel = document.createElement("label");
    payLabel.setAttribute("for", "pay-amount");
    text(payLabel, "Record money received");
    var payInput = document.createElement("input");
    payInput.type = "text";
    payInput.id = "pay-amount";
    payInput.value = view.totals.paid;
    payWrap.appendChild(payLabel);
    payWrap.appendChild(payInput);
    box.appendChild(payWrap);

    var payRow = document.createElement("div");
    payRow.className = "btnrow";
    payRow.appendChild(button("Save the payment", "btn ghost", function () {
      api("POST", "/api/order/payment", { ref: view.order.ref, paid: payInput.value, note: view.order.paymentNote || "" })
        .then(function () { flash("Payment recorded.", "ok"); return reload().then(function () { openOrder(view.order.ref); }); },
          function (err) { flash(err.message, "bad"); });
    }));
    box.appendChild(payRow);

    /* One more line, at today's catalogue price — which is the usual reason an
       order is reopened: she rang back and wants another jar. */
    var addHead = document.createElement("h3");
    addHead.className = "spaced";
    text(addHead, "Add another line");
    box.appendChild(addHead);

    var addNote = document.createElement("p");
    addNote.className = "hint";
    text(addNote, "A line added now takes the price from the catalogue as it stands today. " +
      "The lines already on the order keep the price they were taken at.");
    box.appendChild(addNote);

    var addPick = document.createElement("select");
    addPick.id = "line-product";
    options(addPick, state.products, function (p) { return p.code; },
      function (p) { return p.name + " · " + money(p.price) + (p.availability === "sold-out" ? " · sold out" : ""); },
      state.products.length ? state.products[0].code : "");
    var addField = document.createElement("div");
    addField.className = "field";
    addField.appendChild(addPick);
    box.appendChild(addField);

    var addQty = document.createElement("input");
    addQty.type = "text";
    addQty.id = "line-qty";
    addQty.value = "1";
    addQty.inputMode = "decimal";
    var addQtyField = document.createElement("div");
    addQtyField.className = "field";
    var addQtyLabel = document.createElement("label");
    addQtyLabel.setAttribute("for", "line-qty");
    text(addQtyLabel, "How many");
    addQtyField.appendChild(addQtyLabel);
    addQtyField.appendChild(addQty);
    box.appendChild(addQtyField);

    var addRow = document.createElement("div");
    addRow.className = "btnrow";
    addRow.appendChild(button("Put it on the order", "btn ghost", function () {
      api("POST", "/api/order/line", {
        ref: view.order.ref, code: addPick.value, qty: addQty.value || "0", note: ""
      }).then(function () {
        flash("Line added at today's price.", "ok");
        return reload().then(function () { openOrder(view.order.ref); });
      }, function (err) { flash(err.message, "bad"); });
    }));
    box.appendChild(addRow);

    /* Change the order itself. The prices go back exactly as they came, so
       editing an order after a price change does not reprice it. */
    var editHead = document.createElement("h3");
    editHead.className = "spaced";
    text(editHead, "Change the order");
    box.appendChild(editHead);

    var fields = [
      { id: "edit-wanted", label: "Wanted on", type: "date", value: view.order.wanted },
      { id: "edit-fee", label: "Delivery charge", type: "text", value: view.order.deliveryFee },
      { id: "edit-discount", label: "Discount", type: "text", value: view.order.discount },
      { id: "edit-address", label: "Address", type: "text", value: view.order.customer.address }
    ];
    var inputs = {};
    fields.forEach(function (field) {
      var wrap = document.createElement("div");
      wrap.className = "field";
      var label = document.createElement("label");
      label.setAttribute("for", field.id);
      text(label, field.label);
      var input = document.createElement("input");
      input.type = field.type;
      input.id = field.id;
      input.value = field.value || "";
      inputs[field.id] = input;
      wrap.appendChild(label);
      wrap.appendChild(input);
      box.appendChild(wrap);
    });

    var slotWrap = document.createElement("div");
    slotWrap.className = "field";
    var slotLabel = document.createElement("label");
    slotLabel.setAttribute("for", "edit-slot");
    text(slotLabel, "Part of the day");
    var slotPick = document.createElement("select");
    slotPick.id = "edit-slot";
    options(slotPick, state.reference.slots, function (s) { return s.value; },
      function (s) { return s.name; }, view.order.slot);
    slotWrap.appendChild(slotLabel);
    slotWrap.appendChild(slotPick);
    box.appendChild(slotWrap);

    var editRow = document.createElement("div");
    editRow.className = "btnrow";
    editRow.appendChild(button("Save the change", "btn ghost", function () {
      var changed = JSON.parse(JSON.stringify(view.order));
      changed.wanted = inputs["edit-wanted"].value;
      changed.slot = slotPick.value;
      changed.deliveryFee = inputs["edit-fee"].value || "0";
      changed.discount = inputs["edit-discount"].value || "0";
      changed.customer.address = inputs["edit-address"].value;

      api("PUT", "/api/order", changed).then(function () {
        flash("The order is changed.", "ok");
        return reload().then(function () { openOrder(view.order.ref); });
      }, function (err) { flash(err.message, "bad"); });
    }));
    box.appendChild(editRow);

    /* Stages it can move to. */
    if (view.next.length) {
      var stageHead = document.createElement("h3");
      stageHead.className = "spaced";
      text(stageHead, "Move it on");
      box.appendChild(stageHead);

      var stageRow = document.createElement("div");
      stageRow.className = "btnrow";
      view.next.forEach(function (status) {
        stageRow.appendChild(button(status, "btn ghost", function () {
          api("POST", "/api/order/status", { ref: view.order.ref, status: status })
            .then(function () { flash("Moved to " + status + ".", "ok"); return reload().then(function () { openOrder(view.order.ref); }); },
              function (err) { flash(err.message, "bad"); });
        }));
      });
      box.appendChild(stageRow);
    }

    /* Problems. */
    var problemHead = document.createElement("h3");
    problemHead.className = "spaced";
    text(problemHead, "Before you send anything");
    box.appendChild(problemHead);
    var problemList = document.createElement("ul");
    problemList.className = "flags";
    box.appendChild(problemList);
    problems(problemList, view.problems, "Nothing wrong with this order.");

    /* Messages. */
    var messageHead = document.createElement("h3");
    messageHead.className = "spaced";
    text(messageHead, "Messages to send");
    box.appendChild(messageHead);

    messages.forEach(function (message) {
      var block = document.createElement("div");
      block.className = "msgblock";
      var head = document.createElement("h3");
      text(head, message.title);
      block.appendChild(head);

      if (!message.available) {
        var why = document.createElement("p");
        why.className = "why";
        text(why, message.why || "not available");
        block.appendChild(why);
        box.appendChild(block);
        return;
      }

      var pre = document.createElement("pre");
      pre.className = "message";
      text(pre, message.text);
      block.appendChild(pre);

      var row = document.createElement("div");
      row.className = "btnrow";
      linkRow(row, message.whatsapp, "Open the chat");
      if (message.mail && message.mail.url) { linkRow(row, message.mail, "Open an email"); }
      block.appendChild(row);
      linkMeta(block, message.whatsapp);

      box.appendChild(block);
    });

    var footRow = document.createElement("div");
    footRow.className = "btnrow";
    footRow.appendChild(button("Take this order out of the book", "btn ghost", function () {
      if (!window.confirm("Cancelling keeps the record and is almost always better. Take " +
        view.order.ref + " out of the book altogether?")) { return; }
      api("DELETE", "/api/order?ref=" + encodeURIComponent(view.order.ref)).then(function () {
        pickedOrder = null;
        flash("Taken out of the book.", "ok");
        empty($("order-detail"));
        text($("order-title"), "Pick an order");
        return reload();
      }, function (err) { flash(err.message, "bad"); });
    }));
    box.appendChild(footRow);
  }

  /* draftAmount is the ONE place this page multiplies a price by a quantity,
     and it exists only so that a shopkeeper sees a figure while typing an order
     that is not in the book yet. It is labelled a preview on screen.
     Every figure for an order that has been saved comes from the tool, which
     works in whole paise or centavos; nothing here is ever stored, sent to a
     customer, or compared against the tool's own arithmetic. */
  function draftAmount(line) {
    var qty = Math.round(parseFloat(line.qty) * 1000);
    var price = Math.round(parseFloat(line.unitPrice) * 100);
    var minor = Math.round((qty * price) / 1000);
    var sign = minor < 0 ? "-" : "";
    minor = Math.abs(minor);
    return sign + Math.floor(minor / 100) + "." + String(minor % 100).padStart(2, "0");
  }

  /* ───────── take an order ───────── */
  function drawNew() {
    var fulfilment = $("new-fulfilment");
    if (!fulfilment.options.length) {
      options(fulfilment, state.reference.fulfilments, function (f) { return f; }, function (f) { return f; }, "delivery");
      options($("new-slot"), state.reference.slots, function (s) { return s.value; }, function (s) { return s.name; }, "");
      $("new-wanted").value = state.today;
    }
    drawDraftLines();
  }

  function drawDraftLines() {
    var body = $("new-lines");
    empty(body);

    if (!draftLines.length) {
      emptyRow(body, "Nothing on this order yet.", 5);
    }

    draftLines.forEach(function (line, index) {
      var row = document.createElement("tr");

      var pick = document.createElement("select");
      options(pick, state.products, function (p) { return p.code; },
        function (p) {
          return p.name + (p.packAmount && p.packMeasure ? " · " + packLabel(p) : "") +
            " · " + money(p.price) + (p.availability === "sold-out" ? " · sold out" : "");
        }, line.code);
      pick.addEventListener("change", function () { draftLines[index].code = pick.value; drawDraftTotals(); });
      cell(row, pick, "wide");

      var qty = document.createElement("input");
      qty.type = "text";
      qty.className = "num";
      qty.inputMode = "decimal";
      qty.value = line.qty;
      qty.addEventListener("input", function () { draftLines[index].qty = qty.value; drawDraftTotals(); });
      cell(row, qty, "num");

      var note = document.createElement("input");
      note.type = "text";
      note.value = line.note;
      note.addEventListener("input", function () { draftLines[index].note = note.value; });
      cell(row, note);

      var product = findProduct(line.code);
      cell(row, product ? money(draftAmount({ qty: line.qty || "0", unitPrice: product.price })) : "—", "num");

      cell(row, button("Take off", "link", function () {
        draftLines.splice(index, 1);
        drawDraftLines();
      }));

      body.appendChild(row);
    });

    drawDraftTotals();
  }

  function packLabel(product) {
    if (!product.packAmount || !product.packMeasure) { return ""; }
    return product.packAmount + " " + product.packMeasure;
  }

  function findProduct(code) {
    var found = null;
    state.products.forEach(function (p) { if (p.code === code) { found = p; } });
    return found;
  }

  /* The draft total is shown from the lines on screen so the shopkeeper sees a
     figure while typing. It is a preview: the figure that goes in the book is
     worked out by the binary when the order is saved, and the two are compared
     the moment it is. */
  function drawDraftTotals() {
    var subtotal = 0;
    var items = 0;
    draftLines.forEach(function (line) {
      var product = findProduct(line.code);
      if (!product) { return; }
      var qty = parseFloat(line.qty) || 0;
      if (qty <= 0) { return; }
      items += qty;
      subtotal += Math.round(Math.round(qty * 1000) * Math.round(parseFloat(product.price) * 100) / 1000);
    });
    var fee = Math.round((parseFloat($("new-fee").value) || 0) * 100);
    var discount = Math.round((parseFloat($("new-discount").value) || 0) * 100);
    var total = Math.max(0, subtotal + fee - discount);

    text($("new-total"), money((total / 100).toFixed(2)));
    text($("new-sub"), draftLines.length + " line(s) · " + items + " item(s) — this is a preview; " +
      "the tool works the figures out again when the order goes in the book");

    var body = $("new-totals");
    empty(body);
    [["Items", subtotal], ["Delivery", fee], ["Discount", -discount]].forEach(function (pair) {
      if (!pair[1]) { return; }
      var row = document.createElement("tr");
      headCell(row, pair[0]);
      cell(row, money((pair[1] / 100).toFixed(2)), "num");
      body.appendChild(row);
    });

    $("new-address-field").hidden = $("new-fulfilment").value !== "delivery";
  }

  ["new-fee", "new-discount"].forEach(function (id) {
    $(id).addEventListener("input", drawDraftTotals);
  });
  $("new-fulfilment").addEventListener("change", drawDraftTotals);

  $("new-add-line").addEventListener("click", function () {
    if (!state.products.length) {
      flash("Put something in the catalogue first.", "bad");
      return;
    }
    draftLines.push({ code: state.products[0].code, qty: "1", note: "" });
    drawDraftLines();
  });

  $("new-save").addEventListener("click", function () {
    var order = {
      customer: {
        name: $("new-name").value,
        phone: $("new-phone").value,
        email: $("new-email").value,
        address: $("new-address").value
      },
      fulfilment: $("new-fulfilment").value,
      wanted: $("new-wanted").value,
      slot: $("new-slot").value,
      deliveryFee: $("new-fee").value || "0",
      discount: $("new-discount").value || "0",
      discountReason: $("new-discount-reason").value,
      note: $("new-note").value,
      lines: draftLines.map(function (line) {
        return { code: line.code, qty: line.qty || "0", note: line.note, price: null };
      })
    };

    api("POST", "/api/order", order).then(function (answer) {
      draftLines = [];
      ["new-name", "new-phone", "new-email", "new-address", "new-note", "new-discount-reason"].forEach(function (id) {
        $(id).value = "";
      });
      $("new-fee").value = "0";
      $("new-discount").value = "0";
      flash(answer.order.order.ref + " is in the book.", "ok");
      return reload().then(function () {
        show("orders");
        openOrder(answer.order.order.ref);
      });
    }, function (err) { flash(err.message, "bad"); });
  });

  /* ───────── catalogue ───────── */
  function drawCatalogue() {
    if (!$("p-measure").options.length) {
      options($("p-measure"), state.reference.measures, function (m) { return m; },
        function (m) { return m === "" ? "no pack size" : m; }, "g");
      options($("p-availability"), state.reference.availabilities, function (a) { return a; },
        function (a) { return a; }, "available");
    }

    var body = $("catalogue-rows");
    empty(body);
    if (!state.products.length) {
      emptyRow(body, "Nothing in the catalogue yet.", 5);
    }

    state.products.forEach(function (product) {
      var row = document.createElement("tr");
      if (editingProduct === product.code) { row.className = "picked"; }

      var holder = document.createElement("div");
      holder.appendChild(button(product.name || product.code, "link", function () { editProduct(product); }));
      var meta = document.createElement("span");
      meta.className = "meta";
      text(meta, product.code + (product.unit ? " · a " + product.unit : "") +
        (product.hidden ? " · kept off the shared catalogue" : ""));
      holder.appendChild(meta);
      cell(row, holder);

      cell(row, money(product.price), "num");
      cell(row, perStandard(product), "num");

      var state1 = document.createElement("div");
      state1.appendChild(chip(product.availability,
        product.availability === "sold-out" ? "bad" : (product.availability === "made-to-order" ? "warn" : "ok")));
      if (product.noticeDays) { state1.appendChild(chip(product.noticeDays + "d notice")); }
      cell(row, state1);

      cell(row, button("Remove", "link", function () {
        if (!window.confirm("Remove " + (product.name || product.code) +
          " from the catalogue? Orders already taken keep their own copy and do not change.")) { return; }
        api("DELETE", "/api/product?code=" + encodeURIComponent(product.code)).then(function () {
          flash("Removed from the catalogue.", "ok");
          return reload();
        }, function (err) { flash(err.message, "bad"); });
      }));

      body.appendChild(row);
    });

    problems($("catalogue-problems"), state.catalogueProblems, "Nothing wrong with the catalogue.");
  }

  /* The per-standard figure is not recomputed here: the tool sends it with the
     catalogue CSV and the share text, and the page shows what the pack says. */
  function perStandard(product) {
    if (!product.packAmount || product.packAmount === "0" || !product.packMeasure) { return "—"; }
    var per = product.packMeasure === "piece" ? 1 : 1000;
    var amount = parseFloat(product.packAmount);
    if (!amount) { return "—"; }
    var price = Math.round(parseFloat(product.price) * 100);
    var value = Math.round((price * per) / amount);
    var name = product.packMeasure === "piece" ? "piece" : (product.packMeasure === "ml" ? "litre" : "kg");
    return money((value / 100).toFixed(2)) + " a " + name;
  }

  function editProduct(product) {
    editingProduct = product.code;
    text($("product-title"), "Edit " + (product.name || product.code));
    $("p-code").value = product.code;
    $("p-name").value = product.name;
    $("p-price").value = product.price;
    $("p-unit").value = product.unit;
    $("p-pack").value = product.packAmount === "0" ? "" : product.packAmount;
    $("p-measure").value = product.packMeasure;
    $("p-availability").value = product.availability;
    $("p-notice").value = product.noticeDays;
    $("p-min").value = product.minQty;
    $("p-note").value = product.note;
    $("p-hidden").checked = !!product.hidden;
    text($("p-per"), "");
    drawCatalogue();
  }

  $("p-new").addEventListener("click", function () {
    editingProduct = null;
    text($("product-title"), "Add a product");
    ["p-code", "p-name", "p-price", "p-unit", "p-pack", "p-note"].forEach(function (id) { $(id).value = ""; });
    $("p-notice").value = "0";
    $("p-min").value = "0";
    $("p-hidden").checked = false;
    text($("p-per"), "");
    drawCatalogue();
  });

  $("p-save").addEventListener("click", function () {
    var product = {
      code: $("p-code").value,
      name: $("p-name").value,
      unit: $("p-unit").value,
      price: $("p-price").value || "0",
      packAmount: $("p-pack").value || "0",
      packMeasure: $("p-measure").value,
      availability: $("p-availability").value,
      noticeDays: parseInt($("p-notice").value, 10) || 0,
      minQty: $("p-min").value || "0",
      note: $("p-note").value,
      hidden: $("p-hidden").checked
    };

    api("PUT", "/api/product", product).then(function (answer) {
      editingProduct = answer.product.code;
      flash((answer.product.name || answer.product.code) + " saved.", "ok");
      if (answer.perStandard && answer.perStandard.shown) {
        text($("p-per"), "That works out at " + money(answer.perStandard.amount) +
          " a " + answer.perStandard.unit + ", which is what the shared catalogue shows beside the price.");
      } else {
        text($("p-per"), "No pack size, so the shared catalogue shows the price alone.");
      }
      return reload();
    }, function (err) { flash(err.message, "bad"); });
  });

  /* ───────── share ───────── */

  /* Asked for again whenever this panel is opened, never kept: the catalogue
     text is only right at the moment it is built. */
  function refreshShare() {
    return api("GET", "/api/share").then(function (share) {
      state.share = share;
      drawShare();
    }, function (err) { flash(err.message, "bad"); });
  }

  function drawShare() {
    var share = state.share;
    text($("share-text"), share.text);
    text($("share-chars"), share.characters + " characters");
    text($("share-fit"), share.fitsStatus
      ? "Short enough for a status (" + share.statusLimit + " characters)."
      : "Too long for a status (" + share.statusLimit + " characters) — send it as a message or a broadcast.");

    var body = $("share-table");
    empty(body);
    [["Products listed", share.listed], ["Sold out", share.soldOut], ["Kept off", share.hidden],
      ["Lines", share.lines], ["Prices from", share.pricesChangedOn || "not recorded"],
      ["Fits one message", share.fitsMessage ? "yes" : "no"]].forEach(function (pair) {
      var row = document.createElement("tr");
      headCell(row, pair[0]);
      cell(row, pair[1], "num");
      body.appendChild(row);
    });

    var links = $("share-links");
    empty(links);
    linkRow(links, share.link, "Send it to one customer");
    linkMeta(links, share.link);
    problems($("share-problems"), share.problems, "Nothing wrong with the catalogue.");
  }

  /* ───────── the shop ───────── */
  function drawShop() {
    var shop = state.shop;
    options($("s-currency"), state.reference.currencies, function (c) { return c; },
      function (c) { return c; }, shop.currency);

    $("s-name").value = shop.name;
    $("s-area").value = shop.area;
    $("s-dial").value = shop.dialCode;
    $("s-phone").value = shop.phone;
    $("s-email").value = shop.email;
    $("s-payto").value = shop.payTo;
    $("s-how").value = shop.howToOrder;
    $("s-closing").value = shop.closing;
    $("s-link").value = shop.limits.safeLinkCharacters;
    $("s-status").value = shop.limits.statusCharacters;
    $("s-message").value = shop.limits.messageCharacters;

    text($("where"), "Everything is in " + state.dataFile + " on this machine, written so only your own " +
      "user can read it. It holds your customers' names, numbers and addresses, and nothing in this tool " +
      "sends any of it anywhere.");

    var samples = $("sample-buttons");
    empty(samples);
    state.reference.markets.forEach(function (market) {
      samples.appendChild(button("Load the worked shop in " + market.name, "btn ghost", function () {
        if (!window.confirm("This replaces the shop, the catalogue and every order with a worked example. " +
          "Carry on?")) { return; }
        api("POST", "/api/sample", { market: market.value }).then(function (next) {
          state = next;
          draftLines = [];
          pickedOrder = null;
          editingProduct = null;
          flash("Loaded the worked shop in " + market.name + ".", "ok");
          drawAll();
        }, function (err) { flash(err.message, "bad"); });
      }));
    });
  }

  $("s-save").addEventListener("click", function () {
    var shop = {
      name: $("s-name").value,
      area: $("s-area").value,
      currency: $("s-currency").value,
      dialCode: $("s-dial").value,
      phone: $("s-phone").value,
      email: $("s-email").value,
      payTo: $("s-payto").value,
      howToOrder: $("s-how").value,
      closing: $("s-closing").value,
      limits: {
        safeLinkCharacters: parseInt($("s-link").value, 10) || 0,
        statusCharacters: parseInt($("s-status").value, 10) || 0,
        messageCharacters: parseInt($("s-message").value, 10) || 0
      }
    };
    api("PUT", "/api/shop", shop).then(function () {
      flash("The shop is saved.", "ok");
      return reload();
    }, function (err) { flash(err.message, "bad"); });
  });

  $("clear-book").addEventListener("click", function () {
    if (!window.confirm("Take every order out of the book? The shop and the catalogue stay.")) { return; }
    api("POST", "/api/clear").then(function (next) {
      state = next;
      pickedOrder = null;
      flash("The order book is empty.", "ok");
      drawAll();
    }, function (err) { flash(err.message, "bad"); });
  });

  /* ───────── wiring ───────── */
  function drawAll() {
    drawToday();
    drawOrders();
    drawNew();
    drawCatalogue();
    drawShare();
    drawShop();
  }

  function reload() {
    return api("GET", "/api/state").then(function (next) {
      state = next;
      drawAll();
    }, function (err) { flash(err.message, "bad"); });
  }

  api("GET", "/api/state").then(function (next) {
    state = next;
    drawAll();
    var remembered = null;
    try { remembered = localStorage.getItem("cob-tab"); } catch (e) { remembered = null; }
    show(PANELS.indexOf(remembered) >= 0 ? remembered : "today");
  }, function (err) {
    flash("Could not reach the tool on this machine: " + err.message, "bad");
  });
}());
