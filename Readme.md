# Candidate Take-Home Exercise – EV Charger TOU Pricing API

## Objective

Design a **data schema and structure** to store **Time-of-Use (TOU) pricing** for individual **electric vehicle (EV) chargers**, along with:

- An **API specification**
- A **service implementation** to retrieve this data.

---

# Background

**Time-of-Use (TOU) pricing** is a system used by energy providers to vary electricity costs based on the **time of day**.

This pricing structure incentivizes consumers to use electricity during **off-peak hours**, reducing strain on the electric grid.

For **EV chargers**, TOU pricing encourages owners to charge vehicles at optimal times, helping balance energy demand.

This exercise involves creating:

- A **database schema**
- An **API**

to store and expose **TOU pricing data** for **individual EV chargers at various locations and times**.

> Pricing for EV charging is represented as a rate of **$/kWh**, similar to electrical utility bills.

---

# Example Pricing Schedule

| Time Period | Price ($/kWh) |
|-------------|---------------|
| 00:00 - 06:00 | 0.15 |
| 06:00 - 12:00 | 0.20 |
| 12:00 - 14:00 | 0.25 |
| 14:00 - 18:00 | 0.30 |
| 18:00 - 20:00 | 0.25 |
| 20:00 - 22:00 | 0.20 |
| 22:00 - 00:00 | 0.15 |

---

# Requirements

## 1. Database Schema Design

Design a **relational database schema** to store TOU pricing data for an **EV charging system**.

### Key Requirements

#### Charger-Specific Pricing

- Each **charger can have its own TOU rates**
- Pricing data should be linked to **specific charging stations**
- Pricing should **not be generalized by region**

#### Pricing for Hours in a Day

Each pricing period applies to **specific hours within a day**.

For this exercise:

- You **do not need to consider weekday vs weekend variation**

---

### Suggested Schema

Consider how you might organize tables to represent:

- Individual **charging stations**
- **Pricing schedules**
- **Time periods**
- Associated **price per kWh**

Your schema should:

- Be **normalized**
- Support **efficient querying**

---

# 2. API Design

Design a **RESTful API** that allows clients to retrieve **TOU pricing data**.

The API should support queries using:

- **EV charging station**
- **date**
- **time**

---

## Key Requirements

### Endpoints

Design endpoints to:

- **Retrieve TOU pricing information**
- **Update TOU pricing information**

---

### Request / Response Contracts

Each operation should define:

- Request structure
- Response structure

Your response format should include:

- Charger identifiers
- Pricing periods
- Start and end times
- Cost per kWh

---

# 3. Build a Service That Implements the API

You must implement a backend service that exposes the API.

### Preferred Language

- **Go (Golang)**

However, you may use **any common programming language** if preferred.

---

### Code Submission

Upload your code to a **public GitHub repository** and share the link.

---

# Additional Considerations (Optional)

## 1. Time Zones

Charging stations may exist across **different time zones**.

Consider how your system would:

- Store timezone information
- Interpret time-based pricing correctly
- Handle requests coming from different regions

---

## 2. Bulk Updates

Suggest a way to perform **bulk updates for TOU pricing** across multiple charging stations.

Possible strategies may include:

- Batch APIs
- Admin bulk update endpoints
- Background jobs
- CSV uploads